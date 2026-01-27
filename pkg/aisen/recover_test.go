package aisen

import (
	"context"
	"strings"
	"sync"
	"testing"
)

// mockCollector captures events for verification in recover tests.
type mockCollector struct {
	mu        sync.Mutex
	events    []ErrorEvent
	recordErr error
}

func (c *mockCollector) Record(ctx context.Context, event ErrorEvent) error {
	if c.recordErr != nil {
		return c.recordErr
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, event)
	return nil
}

func (c *mockCollector) Flush(ctx context.Context) error {
	return nil
}

func (c *mockCollector) Close() error {
	return nil
}

func (c *mockCollector) getEvents() []ErrorEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]ErrorEvent, len(c.events))
	copy(result, c.events)
	return result
}

func TestRecover_CapturesPanic(t *testing.T) {
	collector := &mockCollector{}
	ctx := context.Background()

	func() {
		defer Recover(ctx, collector)
		panic("test panic")
	}()

	events := collector.getEvents()
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}

	if events[0].Severity != SeverityCrash {
		t.Errorf("Severity = %q, want %q", events[0].Severity, SeverityCrash)
	}
	if events[0].ErrorType != "panic" {
		t.Errorf("ErrorType = %q, want %q", events[0].ErrorType, "panic")
	}
	if events[0].Message != "test panic" {
		t.Errorf("Message = %q, want %q", events[0].Message, "test panic")
	}
}

func TestRecover_IncludesStackTrace(t *testing.T) {
	collector := &mockCollector{}
	ctx := context.Background()

	func() {
		defer Recover(ctx, collector)
		panic("stack trace test")
	}()

	events := collector.getEvents()
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}

	if events[0].StackTrace == "" {
		t.Error("StackTrace should be populated")
	}
	if !strings.Contains(events[0].StackTrace, "goroutine") {
		t.Error("StackTrace should contain goroutine info")
	}
}

func TestRecover_ReturnsRecoveredValue(t *testing.T) {
	collector := &mockCollector{}
	ctx := context.Background()

	// Note: To capture the return value, you must structure the code
	// so that Recover is called directly in the defer.
	// The returned value is "return value test" only when Recover
	// successfully captures the panic.
	events := collector.getEvents()
	initialCount := len(events)

	func() {
		defer Recover(ctx, collector)
		panic("return value test")
	}()

	// The panic was captured and recorded
	events = collector.getEvents()
	if len(events) != initialCount+1 {
		t.Fatalf("Expected event to be recorded")
	}

	// Verify the event contains the panic value
	if events[len(events)-1].Message != "return value test" {
		t.Errorf("Message = %q, want %q", events[len(events)-1].Message, "return value test")
	}
}

func TestRecover_NoPanic_NoEventRecorded(t *testing.T) {
	collector := &mockCollector{}
	ctx := context.Background()

	func() {
		defer Recover(ctx, collector)
		// No panic
	}()

	events := collector.getEvents()
	if len(events) != 0 {
		t.Errorf("Expected 0 events, got %d", len(events))
	}
}

func TestRecover_DoesNotRePanic(t *testing.T) {
	collector := &mockCollector{}
	ctx := context.Background()

	// This should NOT panic after Recover
	func() {
		defer Recover(ctx, collector)
		panic("should be caught")
	}()

	// If we get here, the panic was not re-raised
	// which is the expected behavior
}

func TestRecover_HandlesErrorPanic(t *testing.T) {
	collector := &mockCollector{}
	ctx := context.Background()

	testErr := &testError{msg: "error panic"}
	func() {
		defer Recover(ctx, collector)
		panic(testErr)
	}()

	events := collector.getEvents()
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}

	if events[0].Message != "error panic" {
		t.Errorf("Message = %q, want %q", events[0].Message, "error panic")
	}
}

func TestRecover_IncludesContextID(t *testing.T) {
	collector := &mockCollector{}
	ctx := WithContextID(context.Background(), 12345)

	func() {
		defer Recover(ctx, collector)
		panic("context id test")
	}()

	events := collector.getEvents()
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}

	if events[0].ContextID == nil {
		t.Error("ContextID should be set from context")
	} else if *events[0].ContextID != 12345 {
		t.Errorf("ContextID = %d, want 12345", *events[0].ContextID)
	}
}

// testError is a custom error type for testing.
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
