package aisen

import (
	"testing"
	"time"
)

func TestErrorEventCreation(t *testing.T) {
	contextID := uint64(12345)
	turnDepth := 7
	tokensWasted := int64(500)

	event := ErrorEvent{
		EventID:     "evt-123",
		Timestamp:   time.Now(),
		Fingerprint: "abc123def456",
		Severity:    SeverityError,
		ErrorType:   "timeout",
		Message:     "connection timed out",
		StackTrace:  "goroutine 1 [running]:\nmain.main()\n\t/app/main.go:10",
		Operation:   "tool",
		OperationID: "call-456",
		AgentName:   "researcher",
		ToolName:    "WebSearch",
		ToolArgs:    `{"query": "test"}`,
		ContextID:   &contextID,
		TurnDepth:   &turnDepth,
		SystemState: &SystemState{
			MemoryBytes:    1024 * 1024 * 100,
			GoroutineCount: 42,
			UptimeMs:       60000,
			HostName:       "worker-1",
		},
		TokensWasted: &tokensWasted,
		Metadata: map[string]string{
			"request_id": "req-789",
		},
	}

	// Verify required fields
	if event.EventID != "evt-123" {
		t.Errorf("EventID = %q, want %q", event.EventID, "evt-123")
	}
	if event.Severity != SeverityError {
		t.Errorf("Severity = %q, want %q", event.Severity, SeverityError)
	}
	if event.ErrorType != "timeout" {
		t.Errorf("ErrorType = %q, want %q", event.ErrorType, "timeout")
	}

	// Verify optional pointer fields
	if event.ContextID == nil || *event.ContextID != 12345 {
		t.Errorf("ContextID = %v, want 12345", event.ContextID)
	}
	if event.TurnDepth == nil || *event.TurnDepth != 7 {
		t.Errorf("TurnDepth = %v, want 7", event.TurnDepth)
	}
	if event.SystemState == nil {
		t.Error("SystemState is nil, want non-nil")
	} else if event.SystemState.GoroutineCount != 42 {
		t.Errorf("SystemState.GoroutineCount = %d, want 42", event.SystemState.GoroutineCount)
	}
}

func TestSeverityConstants(t *testing.T) {
	tests := []struct {
		severity Severity
		want     string
	}{
		{SeverityWarning, "warning"},
		{SeverityError, "error"},
		{SeverityCrash, "crash"},
	}

	for _, tt := range tests {
		if string(tt.severity) != tt.want {
			t.Errorf("Severity constant = %q, want %q", tt.severity, tt.want)
		}
	}
}

func TestErrorEvent_OptionalFieldsNil(t *testing.T) {
	// Test that optional fields can be nil (distinguishing "not set" from "zero value")
	event := ErrorEvent{
		EventID:   "evt-minimal",
		Severity:  SeverityWarning,
		ErrorType: "error",
		Message:   "minimal error",
	}

	if event.ContextID != nil {
		t.Error("ContextID should be nil when not set")
	}
	if event.TurnDepth != nil {
		t.Error("TurnDepth should be nil when not set")
	}
	if event.SystemState != nil {
		t.Error("SystemState should be nil when not set")
	}
	if event.TokensWasted != nil {
		t.Error("TokensWasted should be nil when not set")
	}
}
