package aisen

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// testSink captures events for verification in tests.
type testSink struct {
	mu      sync.Mutex
	events  []ErrorEvent
	writeErr error
}

func (s *testSink) Write(ctx context.Context, event ErrorEvent) error {
	if s.writeErr != nil {
		return s.writeErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
	return nil
}

func (s *testSink) Flush(ctx context.Context) error {
	return nil
}

func (s *testSink) Close() error {
	return nil
}

func (s *testSink) getEvents() []ErrorEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]ErrorEvent, len(s.events))
	copy(result, s.events)
	return result
}

func TestCollector_Record_GeneratesEventID(t *testing.T) {
	sink := &testSink{}
	collector := NewCollector(WithSink(sink))

	event := ErrorEvent{
		Severity:  SeverityError,
		ErrorType: "test",
		Message:   "test error",
	}

	err := collector.Record(context.Background(), event)
	if err != nil {
		t.Fatalf("Record returned error: %v", err)
	}

	events := sink.getEvents()
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}

	if events[0].EventID == "" {
		t.Error("EventID should be generated, got empty string")
	}

	// Should be a UUID format (36 chars with hyphens)
	if len(events[0].EventID) != 36 {
		t.Errorf("EventID length = %d, want 36 (UUID format)", len(events[0].EventID))
	}
}

func TestCollector_Record_SetsTimestamp(t *testing.T) {
	sink := &testSink{}
	collector := NewCollector(WithSink(sink))

	before := time.Now()
	event := ErrorEvent{
		Severity:  SeverityError,
		ErrorType: "test",
		Message:   "test error",
	}

	err := collector.Record(context.Background(), event)
	if err != nil {
		t.Fatalf("Record returned error: %v", err)
	}
	after := time.Now()

	events := sink.getEvents()
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}

	if events[0].Timestamp.Before(before) || events[0].Timestamp.After(after) {
		t.Errorf("Timestamp %v not in expected range [%v, %v]", events[0].Timestamp, before, after)
	}
}

func TestCollector_Record_PreservesExistingTimestamp(t *testing.T) {
	sink := &testSink{}
	collector := NewCollector(WithSink(sink))

	existingTime := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	event := ErrorEvent{
		Timestamp: existingTime,
		Severity:  SeverityError,
		ErrorType: "test",
	}

	err := collector.Record(context.Background(), event)
	if err != nil {
		t.Fatalf("Record returned error: %v", err)
	}

	events := sink.getEvents()
	if !events[0].Timestamp.Equal(existingTime) {
		t.Errorf("Timestamp was modified from %v to %v", existingTime, events[0].Timestamp)
	}
}

func TestCollector_Record_AppliesScrubbing(t *testing.T) {
	sink := &testSink{}
	collector := NewCollector(
		WithSink(sink),
		WithDefaultScrubbing(),
	)

	event := ErrorEvent{
		Severity:  SeverityError,
		ErrorType: "test",
		Message:   "Error with api_key=secret123",
	}

	err := collector.Record(context.Background(), event)
	if err != nil {
		t.Fatalf("Record returned error: %v", err)
	}

	events := sink.getEvents()
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}

	// Message should be scrubbed
	if events[0].Message == event.Message {
		t.Error("Message should have been scrubbed")
	}
	if events[0].Message != "[REDACTED]" && events[0].Message == "Error with api_key=secret123" {
		t.Errorf("Message still contains sensitive data: %q", events[0].Message)
	}
}

func TestCollector_Record_GeneratesFingerprint(t *testing.T) {
	sink := &testSink{}
	collector := NewCollector(WithSink(sink))

	event := ErrorEvent{
		Severity:  SeverityError,
		ErrorType: "timeout",
		Operation: "tool",
		AgentName: "agent1",
		ToolName:  "WebSearch",
	}

	err := collector.Record(context.Background(), event)
	if err != nil {
		t.Fatalf("Record returned error: %v", err)
	}

	events := sink.getEvents()
	if events[0].Fingerprint == "" {
		t.Error("Fingerprint should be generated")
	}

	// Should be 32 hex chars
	if len(events[0].Fingerprint) != 32 {
		t.Errorf("Fingerprint length = %d, want 32", len(events[0].Fingerprint))
	}
}

func TestCollector_Record_ReturnsSinkError(t *testing.T) {
	expectedErr := errors.New("sink error")
	sink := &testSink{writeErr: expectedErr}
	collector := NewCollector(WithSink(sink))

	event := ErrorEvent{
		Severity: SeverityError,
	}

	err := collector.Record(context.Background(), event)
	if err == nil {
		t.Error("Record should return sink error")
	}
	if !errors.Is(err, expectedErr) {
		t.Errorf("Expected error %v, got %v", expectedErr, err)
	}
}

func TestCollector_WithScrubberConfig(t *testing.T) {
	sink := &testSink{}
	cfg := ScrubberConfig{
		MaxMessageSize: 50,
		ScrubMessages:  true,
		FailClosed:     true,
	}
	collector := NewCollector(
		WithSink(sink),
		WithScrubber(cfg),
	)

	longMessage := "This is a very long message that exceeds the configured maximum size limit"
	event := ErrorEvent{
		Severity:  SeverityError,
		ErrorType: "test",
		Message:   longMessage,
	}

	err := collector.Record(context.Background(), event)
	if err != nil {
		t.Fatalf("Record returned error: %v", err)
	}

	events := sink.getEvents()
	if len(events[0].Message) > 70 { // Allow for truncation marker
		t.Errorf("Message should be truncated, length = %d", len(events[0].Message))
	}
}

func TestCollector_Flush(t *testing.T) {
	sink := &testSink{}
	collector := NewCollector(WithSink(sink))

	err := collector.Flush(context.Background())
	if err != nil {
		t.Errorf("Flush returned error: %v", err)
	}
}

func TestCollector_Close(t *testing.T) {
	sink := &testSink{}
	collector := NewCollector(WithSink(sink))

	err := collector.Close()
	if err != nil {
		t.Errorf("Close returned error: %v", err)
	}
}

func TestNewCollector_NilSink(t *testing.T) {
	// Should not panic with nil sink, should use a default
	collector := NewCollector()

	event := ErrorEvent{
		Severity: SeverityError,
	}

	// Should not panic
	_ = collector.Record(context.Background(), event)
}
