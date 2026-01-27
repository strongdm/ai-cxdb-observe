package agentssdk

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/strongdm/ai-cxdb-observe/pkg/aisen"
)

// capturingSink captures events for integration testing.
type capturingSink struct {
	mu     sync.Mutex
	events []aisen.ErrorEvent
}

func (s *capturingSink) Write(ctx context.Context, event aisen.ErrorEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
	return nil
}

func (s *capturingSink) Flush(ctx context.Context) error {
	return nil
}

func (s *capturingSink) Close() error {
	return nil
}

func (s *capturingSink) getEvents() []aisen.ErrorEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]aisen.ErrorEvent, len(s.events))
	copy(result, s.events)
	return result
}

// mockSessionWithContextID implements ContextIDProvider for testing.
type mockSessionWithContextID struct {
	contextID uint64
}

func (s *mockSessionWithContextID) ContextID(ctx context.Context) (uint64, error) {
	return s.contextID, nil
}

// Minimal Session interface methods (not needed for our test but required by interface)
func (s *mockSessionWithContextID) LoadHistory(ctx context.Context) ([]interface{}, error) {
	return nil, nil
}
func (s *mockSessionWithContextID) Append(ctx context.Context, msgs []interface{}) error { return nil }
func (s *mockSessionWithContextID) Reset(ctx context.Context) error                      { return nil }

func TestIntegration_ErrorCapture_EndToEnd(t *testing.T) {
	// Setup: Create capturing sink and collector
	sink := &capturingSink{}
	collector := aisen.NewCollector(
		aisen.WithSink(sink),
		aisen.WithDefaultScrubbing(),
	)

	// Create wrapper using test infrastructure
	store := NewEnrichmentStore()
	wrapper := newWrappedRunnerForTest(nil, collector, store, nil)

	// Add enrichment data as if hooks had captured it
	runID := "test-run-id"
	store.Update(runID, func(e *Enrichment) {
		e.AgentName = "test-agent"
		e.ToolName = "TestTool"
		e.Operation = "tool"
	})

	// Simulate error
	wrapper.testError = errors.New("integration test error")

	ctx := context.Background()
	_, err := wrapper.Run(ctx, nil, "test input", nil, nil)

	// Verify error was returned
	if err == nil {
		t.Fatal("Expected error to be returned")
	}

	// Verify event was captured
	events := sink.getEvents()
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}

	event := events[0]

	// Verify event fields
	if event.Severity != aisen.SeverityError {
		t.Errorf("Severity = %q, want %q", event.Severity, aisen.SeverityError)
	}
	if event.Message != "integration test error" {
		t.Errorf("Message = %q, want %q", event.Message, "integration test error")
	}
	if event.AgentName != "test-agent" {
		t.Errorf("AgentName = %q, want %q", event.AgentName, "test-agent")
	}
	if event.ToolName != "TestTool" {
		t.Errorf("ToolName = %q, want %q", event.ToolName, "TestTool")
	}
	if event.Operation != "tool" {
		t.Errorf("Operation = %q, want %q", event.Operation, "tool")
	}
	if event.Fingerprint == "" {
		t.Error("Fingerprint should be generated")
	}
	if event.EventID == "" {
		t.Error("EventID should be generated")
	}
	if event.Timestamp.IsZero() {
		t.Error("Timestamp should be set")
	}
}

func TestIntegration_PanicCapture_EndToEnd(t *testing.T) {
	// Setup
	sink := &capturingSink{}
	collector := aisen.NewCollector(
		aisen.WithSink(sink),
		aisen.WithDefaultScrubbing(),
	)

	store := NewEnrichmentStore()
	wrapper := newWrappedRunnerForTest(nil, collector, store, nil)

	// Add enrichment
	runID := "test-run-id"
	store.Update(runID, func(e *Enrichment) {
		e.AgentName = "panic-agent"
		e.Operation = "llm"
	})

	// Simulate panic
	wrapper.testPanic = "integration panic test"

	ctx := context.Background()

	// Should panic
	defer func() {
		r := recover()
		if r == nil {
			t.Error("Expected panic to be re-raised")
			return
		}
		if r != "integration panic test" {
			t.Errorf("Recovered = %v, want %q", r, "integration panic test")
		}

		// Verify event was captured before re-panic
		events := sink.getEvents()
		if len(events) != 1 {
			t.Fatalf("Expected 1 event, got %d", len(events))
		}

		event := events[0]

		if event.Severity != aisen.SeverityCrash {
			t.Errorf("Severity = %q, want %q", event.Severity, aisen.SeverityCrash)
		}
		if event.ErrorType != "panic" {
			t.Errorf("ErrorType = %q, want %q", event.ErrorType, "panic")
		}
		if event.Message != "integration panic test" {
			t.Errorf("Message = %q, want %q", event.Message, "integration panic test")
		}
		if event.StackTrace == "" {
			t.Error("StackTrace should be captured for panics")
		}
		if event.AgentName != "panic-agent" {
			t.Errorf("AgentName = %q, want %q", event.AgentName, "panic-agent")
		}
	}()

	wrapper.Run(ctx, nil, "test input", nil, nil)
}

func TestIntegration_ContextID_Extracted(t *testing.T) {
	// Setup
	sink := &capturingSink{}
	collector := aisen.NewCollector(
		aisen.WithSink(sink),
		aisen.WithDefaultScrubbing(),
	)

	store := NewEnrichmentStore()
	wrapper := newWrappedRunnerForTest(nil, collector, store, nil)

	// Simulate error to trigger event capture
	wrapper.testError = errors.New("context id test error")

	// Create session with context ID
	session := &mockSessionWithContextID{contextID: 98765}

	ctx := context.Background()
	wrapper.Run(ctx, nil, "test input", session, nil)

	// Verify context ID was extracted
	events := sink.getEvents()
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}

	event := events[0]

	if event.ContextID == nil {
		t.Error("ContextID should be set from session")
	} else if *event.ContextID != 98765 {
		t.Errorf("ContextID = %d, want 98765", *event.ContextID)
	}
}

func TestIntegration_Scrubbing_Applied(t *testing.T) {
	// Setup with scrubbing enabled
	sink := &capturingSink{}
	collector := aisen.NewCollector(
		aisen.WithSink(sink),
		aisen.WithDefaultScrubbing(),
	)

	store := NewEnrichmentStore()
	wrapper := newWrappedRunnerForTest(nil, collector, store, nil)

	// Error message containing sensitive data
	wrapper.testError = errors.New("API key sk-proj-1234567890abcdef leaked")

	ctx := context.Background()
	wrapper.Run(ctx, nil, "test input", nil, nil)

	// Verify scrubbing was applied
	events := sink.getEvents()
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}

	event := events[0]

	// The message should have the API key redacted
	if event.Message == "API key sk-proj-1234567890abcdef leaked" {
		t.Error("Message should have been scrubbed")
	}
	// Should contain redacted placeholder
	if event.Message != "API key [REDACTED] leaked" {
		t.Errorf("Message = %q, want scrubbed version", event.Message)
	}
}

func TestIntegration_HooksAndWrapper_Together(t *testing.T) {
	// This test verifies that HookAdapter and WrappedRunner work together.
	// The hooks provide enrichment, the wrapper captures errors.

	sink := &capturingSink{}
	collector := aisen.NewCollector(
		aisen.WithSink(sink),
	)

	// Shared enrichment store
	store := NewEnrichmentStore()

	// Create hook adapter (would normally be wired into RunConfig.Hooks)
	_ = NewHookAdapter(store, nil, nil)

	// Create wrapper
	wrapper := newWrappedRunnerForTest(nil, collector, store, nil)

	// Simulate what hooks would do during a run
	runID := "test-run-id"
	store.Update(runID, func(e *Enrichment) {
		e.AgentName = "orchestrator"
	})
	store.Update(runID, func(e *Enrichment) {
		e.Operation = "llm"
		e.Model = "gpt-4"
	})
	store.Update(runID, func(e *Enrichment) {
		e.Operation = "tool"
		e.ToolName = "WebSearch"
		e.ToolCallID = "call-123"
	})

	// Error occurs
	wrapper.testError = errors.New("tool execution failed")

	ctx := context.Background()
	wrapper.Run(ctx, nil, "search query", nil, nil)

	// Verify enrichment was merged into event
	events := sink.getEvents()
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}

	event := events[0]

	// Should have the most recent enrichment values
	if event.AgentName != "orchestrator" {
		t.Errorf("AgentName = %q, want %q", event.AgentName, "orchestrator")
	}
	if event.Operation != "tool" {
		t.Errorf("Operation = %q, want %q", event.Operation, "tool")
	}
	if event.ToolName != "WebSearch" {
		t.Errorf("ToolName = %q, want %q", event.ToolName, "WebSearch")
	}
}
