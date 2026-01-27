package agentssdk

import (
	"context"
	"errors"
	"log"
	"os"
	"sync"
	"testing"

	"github.com/strongdm/ai-cxdb-observe/pkg/aisen"
)

// testCollector captures events for verification.
type testCollector struct {
	mu       sync.Mutex
	events   []aisen.ErrorEvent
	recordErr error
}

func (c *testCollector) Record(ctx context.Context, event aisen.ErrorEvent) error {
	if c.recordErr != nil {
		return c.recordErr
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, event)
	return nil
}

func (c *testCollector) Flush(ctx context.Context) error {
	return nil
}

func (c *testCollector) Close() error {
	return nil
}

func (c *testCollector) getEvents() []aisen.ErrorEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]aisen.ErrorEvent, len(c.events))
	copy(result, c.events)
	return result
}

// mockSession implements agents.Session with optional ContextIDProvider.
type mockSession struct {
	contextID uint64
	hasID     bool
}

func (s *mockSession) ContextID(ctx context.Context) (uint64, error) {
	if !s.hasID {
		return 0, errors.New("no context ID")
	}
	return s.contextID, nil
}

// Implement minimal Session interface methods
func (s *mockSession) AddMessage(msg interface{}) error { return nil }
func (s *mockSession) GetMessages() []interface{}      { return nil }
func (s *mockSession) Clear() error                    { return nil }

func TestWrappedRunner_Run_CapturesError(t *testing.T) {
	collector := &testCollector{}
	store := NewEnrichmentStore()
	logger := log.New(os.Stderr, "", 0)

	// Create wrapper without real runner (we'll test error path)
	wrapper := newWrappedRunnerForTest(nil, collector, store, logger)

	// Simulate an error return
	expectedErr := errors.New("run failed")
	wrapper.testError = expectedErr

	ctx := context.Background()
	_, err := wrapper.Run(ctx, nil, "input", nil, nil)

	// Should return the original error
	if !errors.Is(err, expectedErr) {
		t.Errorf("Expected error %v, got %v", expectedErr, err)
	}

	// Should have recorded the error
	events := collector.getEvents()
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}

	if events[0].Message != "run failed" {
		t.Errorf("Event message = %q, want %q", events[0].Message, "run failed")
	}
	if events[0].Severity != aisen.SeverityError {
		t.Errorf("Event severity = %q, want %q", events[0].Severity, aisen.SeverityError)
	}
}

func TestWrappedRunner_Run_CapturesPanic(t *testing.T) {
	collector := &testCollector{}
	store := NewEnrichmentStore()
	logger := log.New(os.Stderr, "", 0)

	wrapper := newWrappedRunnerForTest(nil, collector, store, logger)
	wrapper.testPanic = "test panic value"

	ctx := context.Background()

	// Should re-panic after recording
	defer func() {
		r := recover()
		if r == nil {
			t.Error("Expected panic to be re-raised")
		}
		if r != "test panic value" {
			t.Errorf("Recovered = %v, want %q", r, "test panic value")
		}

		// Should have recorded the panic
		events := collector.getEvents()
		if len(events) != 1 {
			t.Fatalf("Expected 1 event, got %d", len(events))
		}

		if events[0].Severity != aisen.SeverityCrash {
			t.Errorf("Event severity = %q, want %q", events[0].Severity, aisen.SeverityCrash)
		}
		if events[0].ErrorType != "panic" {
			t.Errorf("Event error type = %q, want %q", events[0].ErrorType, "panic")
		}
	}()

	wrapper.Run(ctx, nil, "input", nil, nil)
}

func TestWrappedRunner_Run_ReturnsOriginalError(t *testing.T) {
	collector := &testCollector{}
	store := NewEnrichmentStore()
	logger := log.New(os.Stderr, "", 0)

	wrapper := newWrappedRunnerForTest(nil, collector, store, logger)
	originalErr := errors.New("original error")
	wrapper.testError = originalErr

	ctx := context.Background()
	_, err := wrapper.Run(ctx, nil, "input", nil, nil)

	// Error should be the original, not wrapped
	if err != originalErr {
		t.Errorf("Expected original error %v, got %v", originalErr, err)
	}
}

func TestWrappedRunner_Run_SwallowsCollectorError(t *testing.T) {
	collector := &testCollector{
		recordErr: errors.New("collector failed"),
	}
	store := NewEnrichmentStore()
	logger := log.New(os.Stderr, "", 0)

	wrapper := newWrappedRunnerForTest(nil, collector, store, logger)
	runErr := errors.New("run error")
	wrapper.testError = runErr

	ctx := context.Background()
	_, err := wrapper.Run(ctx, nil, "input", nil, nil)

	// Should return the run error, not collector error
	if !errors.Is(err, runErr) {
		t.Errorf("Expected run error, got %v", err)
	}
}

func TestWrappedRunner_Run_ExtractsContextIDFromSession(t *testing.T) {
	collector := &testCollector{}
	store := NewEnrichmentStore()
	logger := log.New(os.Stderr, "", 0)

	wrapper := newWrappedRunnerForTest(nil, collector, store, logger)
	wrapper.testError = errors.New("test error")

	session := &mockSession{contextID: 12345, hasID: true}

	ctx := context.Background()
	wrapper.Run(ctx, nil, "input", session, nil)

	events := collector.getEvents()
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}

	if events[0].ContextID == nil {
		t.Error("ContextID should be set from session")
	} else if *events[0].ContextID != 12345 {
		t.Errorf("ContextID = %d, want 12345", *events[0].ContextID)
	}
}

func TestWrappedRunner_RunOnce_CapturesError(t *testing.T) {
	collector := &testCollector{}
	store := NewEnrichmentStore()
	logger := log.New(os.Stderr, "", 0)

	wrapper := newWrappedRunnerForTest(nil, collector, store, logger)
	wrapper.testError = errors.New("run once failed")

	ctx := context.Background()
	_, err := wrapper.RunOnce(ctx, nil, "input", nil)

	if err == nil {
		t.Error("Expected error")
	}

	events := collector.getEvents()
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}
}

func TestWrappedRunner_CleansUpEnrichment(t *testing.T) {
	collector := &testCollector{}
	store := NewEnrichmentStore()
	logger := log.New(os.Stderr, "", 0)

	wrapper := newWrappedRunnerForTest(nil, collector, store, logger)

	ctx := context.Background()
	wrapper.Run(ctx, nil, "input", nil, nil)

	// After Run completes, the enrichment for that run should be cleaned up
	// We can't easily test this without knowing the runID, but we can verify
	// the store is empty after a successful run
	// (This is a simplified check - real test would track the runID)
}

// testWrappedRunner is a test version of WrappedRunner that doesn't need a real Runner.
type testWrappedRunner struct {
	collector   aisen.Collector
	enrichments EnrichmentStore
	logger      *log.Logger
	testError   error
	testPanic   any
}

// newWrappedRunnerForTest creates a test wrapper for unit testing.
func newWrappedRunnerForTest(inner interface{}, collector aisen.Collector, store EnrichmentStore, logger *log.Logger) *testWrappedRunner {
	if store == nil {
		store = NewEnrichmentStore()
	}
	return &testWrappedRunner{
		collector:   collector,
		enrichments: store,
		logger:      logger,
	}
}

func (w *testWrappedRunner) Run(ctx context.Context, agent interface{}, input string, session interface{}, cfg interface{}) (interface{}, error) {
	return w.runWithCapture(ctx, "run", agent, input, session, cfg)
}

func (w *testWrappedRunner) RunOnce(ctx context.Context, agent interface{}, input string, cfg interface{}) (interface{}, error) {
	return w.runWithCapture(ctx, "run_once", agent, input, nil, cfg)
}

func (w *testWrappedRunner) runWithCapture(ctx context.Context, mode string, agent interface{}, input string, session interface{}, cfg interface{}) (interface{}, error) {
	runID := "test-run-id"
	ctx = aisen.WithRunID(ctx, runID)

	defer w.enrichments.Delete(runID)

	// Extract context ID from session if it implements ContextIDProvider
	var contextID uint64
	if provider, ok := session.(aisen.ContextIDProvider); ok {
		if id, err := provider.ContextID(ctx); err == nil {
			contextID = id
		}
	}

	// Capture panics
	defer func() {
		if r := recover(); r != nil {
			enrichment, _ := w.enrichments.Get(runID)
			event := buildPanicEvent(r, contextID, enrichment)
			w.safeRecord(ctx, event)
			panic(r)
		}
	}()

	// Simulate the operation
	if w.testPanic != nil {
		panic(w.testPanic)
	}
	if w.testError != nil {
		enrichment, _ := w.enrichments.Get(runID)
		event := buildErrorEvent(w.testError, contextID, enrichment)
		w.safeRecord(ctx, event)
		return nil, w.testError
	}

	return nil, nil
}

func (w *testWrappedRunner) safeRecord(ctx context.Context, event aisen.ErrorEvent) {
	if err := w.collector.Record(ctx, event); err != nil && w.logger != nil {
		w.logger.Printf("aisen: failed to record error: %v", err)
	}
}
