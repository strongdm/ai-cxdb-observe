package agentssdk

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/strongdm/ai-cxdb-observe/pkg/aisen"
)

func TestBuildErrorEvent_PopulatesFromEnrichment(t *testing.T) {
	enrichment := Enrichment{
		AgentName:   "test-agent",
		ToolName:    "WebSearch",
		ToolCallID:  "call-123",
		Operation:   "tool",
		OperationID: "op-456",
		Model:       "gpt-4",
	}

	err := errors.New("something went wrong")
	event := buildErrorEvent(err, 12345, enrichment)

	if event.AgentName != "test-agent" {
		t.Errorf("AgentName = %q, want %q", event.AgentName, "test-agent")
	}
	if event.ToolName != "WebSearch" {
		t.Errorf("ToolName = %q, want %q", event.ToolName, "WebSearch")
	}
	if event.Operation != "tool" {
		t.Errorf("Operation = %q, want %q", event.Operation, "tool")
	}
	if event.OperationID != "op-456" {
		t.Errorf("OperationID = %q, want %q", event.OperationID, "op-456")
	}
	if event.Severity != aisen.SeverityError {
		t.Errorf("Severity = %q, want %q", event.Severity, aisen.SeverityError)
	}
	if event.ContextID == nil || *event.ContextID != 12345 {
		t.Errorf("ContextID = %v, want 12345", event.ContextID)
	}
}

func TestBuildErrorEvent_SetsMessage(t *testing.T) {
	enrichment := Enrichment{}
	err := errors.New("connection refused")
	event := buildErrorEvent(err, 0, enrichment)

	if event.Message != "connection refused" {
		t.Errorf("Message = %q, want %q", event.Message, "connection refused")
	}
}

func TestBuildErrorEvent_ClassifiesTimeout(t *testing.T) {
	enrichment := Enrichment{}
	event := buildErrorEvent(context.DeadlineExceeded, 0, enrichment)

	if event.ErrorType != "timeout" {
		t.Errorf("ErrorType = %q, want %q", event.ErrorType, "timeout")
	}
}

func TestBuildErrorEvent_ClassifiesCanceled(t *testing.T) {
	enrichment := Enrichment{}
	event := buildErrorEvent(context.Canceled, 0, enrichment)

	if event.ErrorType != "canceled" {
		t.Errorf("ErrorType = %q, want %q", event.ErrorType, "canceled")
	}
}

func TestBuildErrorEvent_ClassifiesWrappedTimeout(t *testing.T) {
	enrichment := Enrichment{}
	wrapped := errors.Join(errors.New("outer"), context.DeadlineExceeded)
	event := buildErrorEvent(wrapped, 0, enrichment)

	if event.ErrorType != "timeout" {
		t.Errorf("ErrorType = %q, want %q for wrapped timeout", event.ErrorType, "timeout")
	}
}

func TestBuildErrorEvent_DefaultErrorType(t *testing.T) {
	enrichment := Enrichment{}
	err := errors.New("some random error")
	event := buildErrorEvent(err, 0, enrichment)

	if event.ErrorType != "error" {
		t.Errorf("ErrorType = %q, want %q", event.ErrorType, "error")
	}
}

func TestBuildErrorEvent_ZeroContextID(t *testing.T) {
	enrichment := Enrichment{}
	err := errors.New("test")
	event := buildErrorEvent(err, 0, enrichment)

	// Zero context ID should be treated as "not set"
	if event.ContextID != nil {
		t.Errorf("ContextID should be nil for zero value, got %v", *event.ContextID)
	}
}

func TestBuildPanicEvent_SeverityIsCrash(t *testing.T) {
	enrichment := Enrichment{}
	event := buildPanicEvent("nil pointer", 0, enrichment)

	if event.Severity != aisen.SeverityCrash {
		t.Errorf("Severity = %q, want %q", event.Severity, aisen.SeverityCrash)
	}
}

func TestBuildPanicEvent_ErrorTypeIsPanic(t *testing.T) {
	enrichment := Enrichment{}
	event := buildPanicEvent("something bad", 0, enrichment)

	if event.ErrorType != "panic" {
		t.Errorf("ErrorType = %q, want %q", event.ErrorType, "panic")
	}
}

func TestBuildPanicEvent_CapturesStackTrace(t *testing.T) {
	enrichment := Enrichment{}
	event := buildPanicEvent("test panic", 0, enrichment)

	if event.StackTrace == "" {
		t.Error("StackTrace should be populated for panic events")
	}

	// Should contain some goroutine info
	if !strings.Contains(event.StackTrace, "goroutine") {
		t.Error("StackTrace should contain goroutine info")
	}
}

func TestBuildPanicEvent_FormatsRecoveredValue(t *testing.T) {
	tests := []struct {
		name      string
		recovered any
		wantMsg   string
	}{
		{"string", "panic message", "panic message"},
		{"error", errors.New("error panic"), "error panic"},
		{"int", 42, "42"},
		{"nil", nil, "<nil>"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := buildPanicEvent(tt.recovered, 0, Enrichment{})

			if event.Message != tt.wantMsg {
				t.Errorf("Message = %q, want %q", event.Message, tt.wantMsg)
			}
		})
	}
}

func TestBuildPanicEvent_PopulatesFromEnrichment(t *testing.T) {
	enrichment := Enrichment{
		AgentName: "panic-agent",
		ToolName:  "FaultyTool",
		Operation: "tool",
	}

	event := buildPanicEvent("crash!", 99999, enrichment)

	if event.AgentName != "panic-agent" {
		t.Errorf("AgentName = %q, want %q", event.AgentName, "panic-agent")
	}
	if event.ToolName != "FaultyTool" {
		t.Errorf("ToolName = %q, want %q", event.ToolName, "FaultyTool")
	}
	if event.ContextID == nil || *event.ContextID != 99999 {
		t.Errorf("ContextID = %v, want 99999", event.ContextID)
	}
}

// TestErrorEventIncludesOperationHistory verifies history is stored in metadata.
func TestErrorEventIncludesOperationHistory(t *testing.T) {
	enrichment := Enrichment{
		AgentName: "test-agent",
	}

	// Record some operations
	enrichment.RecordOperation(OperationRecord{
		Kind:      "llm",
		AgentName: "test-agent",
		LLM:       &LLMOperation{Model: "gpt-4"},
	})
	enrichment.RecordOperation(OperationRecord{
		Kind:      "tool",
		AgentName: "test-agent",
		Tool:      &ToolOperation{Name: "search"},
	})

	err := errors.New("test error")
	event := buildErrorEvent(err, 0, enrichment)

	// Check that metadata contains operation history JSON
	if event.Metadata == nil {
		t.Fatal("Metadata should not be nil")
	}

	historyJSON, ok := event.Metadata["aisen.operation_history_json"]
	if !ok {
		t.Fatal("Metadata should contain aisen.operation_history_json key")
	}

	// Verify it's valid JSON and contains expected operations
	if !strings.Contains(historyJSON, `"kind":"llm"`) {
		t.Errorf("History should contain LLM operation")
	}
	if !strings.Contains(historyJSON, `"kind":"tool"`) {
		t.Errorf("History should contain tool operation")
	}
	if !strings.Contains(historyJSON, "gpt-4") {
		t.Errorf("History should contain model name")
	}
	if !strings.Contains(historyJSON, "search") {
		t.Errorf("History should contain tool name")
	}
}

// TestErrorEventEmptyHistoryOmitsMetadata verifies omission when no operations.
func TestErrorEventEmptyHistoryOmitsMetadata(t *testing.T) {
	enrichment := Enrichment{
		AgentName: "test-agent",
	}
	// No operations recorded

	err := errors.New("test error")
	event := buildErrorEvent(err, 0, enrichment)

	// Metadata should either be nil or not contain the history key
	if event.Metadata != nil {
		if _, ok := event.Metadata["aisen.operation_history_json"]; ok {
			t.Error("Empty history should not create metadata key")
		}
	}
}

// TestOperationHistoryClearedBetweenRuns verifies no cross-run leakage.
func TestOperationHistoryClearedBetweenRuns(t *testing.T) {
	store := NewEnrichmentStore()

	// Run 1: record operations
	store.Update("run-1", func(e *Enrichment) {
		e.RecordOperation(OperationRecord{Kind: "llm", AgentName: "agent1"})
	})

	// Get run-1 enrichment
	enrichment1, _ := store.Get("run-1")
	history1 := enrichment1.GetOperationHistory()
	if len(history1) != 1 {
		t.Fatalf("Run 1 should have 1 operation, got %d", len(history1))
	}

	// Delete run-1 and create run-2
	store.Delete("run-1")
	store.Update("run-2", func(e *Enrichment) {
		e.AgentName = "agent2"
	})

	// Get run-2 enrichment - should have no history
	enrichment2, _ := store.Get("run-2")
	history2 := enrichment2.GetOperationHistory()
	if len(history2) != 0 {
		t.Errorf("Run 2 should have no operations, got %d", len(history2))
	}
}
