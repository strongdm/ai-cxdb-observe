package multi

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/strongdm/ai-cxdb-observe/pkg/aisen"
)

// mockSink is a test sink that tracks calls and can return errors.
type mockSink struct {
	mu       sync.Mutex
	events   []aisen.ErrorEvent
	writeErr error
	flushErr error
	closeErr error
	closed   bool
}

func (s *mockSink) Write(ctx context.Context, event aisen.ErrorEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.writeErr != nil {
		return s.writeErr
	}
	s.events = append(s.events, event)
	return nil
}

func (s *mockSink) Flush(ctx context.Context) error {
	return s.flushErr
}

func (s *mockSink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return s.closeErr
}

func (s *mockSink) getEvents() []aisen.ErrorEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]aisen.ErrorEvent, len(s.events))
	copy(result, s.events)
	return result
}

func (s *mockSink) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func TestMultiSink_ImplementsSinkInterface(t *testing.T) {
	var _ aisen.Sink = NewMultiSink()
}

func TestMultiSink_Write_CallsAllSinks(t *testing.T) {
	sink1 := &mockSink{}
	sink2 := &mockSink{}
	sink3 := &mockSink{}
	multi := NewMultiSink(sink1, sink2, sink3)

	event := aisen.ErrorEvent{
		EventID:   "evt-123",
		Timestamp: time.Now(),
		Severity:  aisen.SeverityError,
	}

	err := multi.Write(context.Background(), event)
	if err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	// All sinks should receive the event
	for i, sink := range []*mockSink{sink1, sink2, sink3} {
		events := sink.getEvents()
		if len(events) != 1 {
			t.Errorf("sink%d: expected 1 event, got %d", i+1, len(events))
		}
		if len(events) > 0 && events[0].EventID != "evt-123" {
			t.Errorf("sink%d: wrong event ID", i+1)
		}
	}
}

func TestMultiSink_Write_AggregatesErrors(t *testing.T) {
	err1 := errors.New("sink1 error")
	err2 := errors.New("sink2 error")
	sink1 := &mockSink{writeErr: err1}
	sink2 := &mockSink{writeErr: err2}
	sink3 := &mockSink{} // No error
	multi := NewMultiSink(sink1, sink2, sink3)

	event := aisen.ErrorEvent{}
	err := multi.Write(context.Background(), event)

	if err == nil {
		t.Fatal("Write should return error when sinks fail")
	}

	// Both errors should be present
	if !errors.Is(err, err1) {
		t.Errorf("Error should contain err1: %v", err)
	}
	if !errors.Is(err, err2) {
		t.Errorf("Error should contain err2: %v", err)
	}
}

func TestMultiSink_Write_ContinuesOnError(t *testing.T) {
	sink1 := &mockSink{writeErr: errors.New("sink1 error")}
	sink2 := &mockSink{} // No error - should still be called
	sink3 := &mockSink{} // No error - should still be called
	multi := NewMultiSink(sink1, sink2, sink3)

	event := aisen.ErrorEvent{EventID: "evt-test"}
	_ = multi.Write(context.Background(), event)

	// sink2 and sink3 should still receive the event
	if len(sink2.getEvents()) != 1 {
		t.Error("sink2 should still receive event after sink1 fails")
	}
	if len(sink3.getEvents()) != 1 {
		t.Error("sink3 should still receive event after sink1 fails")
	}
}

func TestMultiSink_Flush_CallsAllSinks(t *testing.T) {
	err1 := errors.New("flush error 1")
	err2 := errors.New("flush error 2")
	sink1 := &mockSink{flushErr: err1}
	sink2 := &mockSink{flushErr: err2}
	multi := NewMultiSink(sink1, sink2)

	err := multi.Flush(context.Background())

	if err == nil {
		t.Fatal("Flush should return error")
	}
	if !errors.Is(err, err1) || !errors.Is(err, err2) {
		t.Error("Flush should aggregate all errors")
	}
}

func TestMultiSink_Close_CallsAllSinks(t *testing.T) {
	sink1 := &mockSink{}
	sink2 := &mockSink{}
	multi := NewMultiSink(sink1, sink2)

	err := multi.Close()
	if err != nil {
		t.Errorf("Close returned error: %v", err)
	}

	if !sink1.isClosed() {
		t.Error("sink1 should be closed")
	}
	if !sink2.isClosed() {
		t.Error("sink2 should be closed")
	}
}

func TestMultiSink_Close_AggregatesErrors(t *testing.T) {
	err1 := errors.New("close error 1")
	err2 := errors.New("close error 2")
	sink1 := &mockSink{closeErr: err1}
	sink2 := &mockSink{closeErr: err2}
	multi := NewMultiSink(sink1, sink2)

	err := multi.Close()

	if err == nil {
		t.Fatal("Close should return error")
	}
	if !errors.Is(err, err1) || !errors.Is(err, err2) {
		t.Error("Close should aggregate all errors")
	}
}

func TestMultiSink_EmptySinks(t *testing.T) {
	multi := NewMultiSink()

	event := aisen.ErrorEvent{}
	err := multi.Write(context.Background(), event)
	if err != nil {
		t.Errorf("Write with no sinks should return nil, got: %v", err)
	}

	err = multi.Flush(context.Background())
	if err != nil {
		t.Errorf("Flush with no sinks should return nil, got: %v", err)
	}

	err = multi.Close()
	if err != nil {
		t.Errorf("Close with no sinks should return nil, got: %v", err)
	}
}
