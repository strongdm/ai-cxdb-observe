package async

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/strongdm/ai-cxdb-observe/pkg/aisen"
)

// slowSink is a test sink that can be slow and tracks events.
type slowSink struct {
	mu       sync.Mutex
	events   []aisen.ErrorEvent
	delay    time.Duration
}

func (s *slowSink) Write(ctx context.Context, event aisen.ErrorEvent) error {
	if s.delay > 0 {
		time.Sleep(s.delay)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
	return nil
}

func (s *slowSink) Flush(ctx context.Context) error {
	return nil
}

func (s *slowSink) Close() error {
	return nil
}

func (s *slowSink) getEvents() []aisen.ErrorEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]aisen.ErrorEvent, len(s.events))
	copy(result, s.events)
	return result
}

func TestAsyncSink_ImplementsSinkInterface(t *testing.T) {
	inner := &slowSink{}
	var _ aisen.Sink = NewAsyncSink(inner)
}

func TestAsyncSink_Write_ReturnsImmediately(t *testing.T) {
	inner := &slowSink{delay: 100 * time.Millisecond}
	sink := NewAsyncSink(inner, WithQueueSize(100))
	defer sink.Close()

	event := aisen.ErrorEvent{EventID: "evt-1"}

	start := time.Now()
	err := sink.Write(context.Background(), event)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	// Write should return immediately (much less than the inner sink's delay)
	if elapsed > 10*time.Millisecond {
		t.Errorf("Write took %v, should return in <10ms", elapsed)
	}
}

func TestAsyncSink_DropsOldest_WhenQueueFull(t *testing.T) {
	inner := &slowSink{delay: 50 * time.Millisecond} // Slow enough to fill queue
	var droppedCount atomic.Int32
	sink := NewAsyncSink(inner,
		WithQueueSize(2),
		WithOnDropped(func(count int) {
			droppedCount.Add(int32(count))
		}),
	)

	// Write 5 events quickly - queue size is 2, so we'll drop some
	for i := 0; i < 5; i++ {
		event := aisen.ErrorEvent{EventID: "evt-" + string(rune('0'+i))}
		sink.Write(context.Background(), event)
	}

	// Wait for processing and close
	time.Sleep(50 * time.Millisecond)
	sink.Close()

	// Should have dropped some events
	dropped := droppedCount.Load()
	if dropped == 0 {
		t.Error("Should have dropped some events when queue is full")
	}
}

func TestAsyncSink_OnDropped_Called(t *testing.T) {
	inner := &slowSink{delay: 100 * time.Millisecond}
	var droppedCalled atomic.Bool
	var droppedCount atomic.Int32

	sink := NewAsyncSink(inner,
		WithQueueSize(1),
		WithOnDropped(func(count int) {
			droppedCalled.Store(true)
			droppedCount.Add(int32(count))
		}),
	)

	// Fill the queue and trigger drop
	for i := 0; i < 10; i++ {
		sink.Write(context.Background(), aisen.ErrorEvent{EventID: "evt"})
	}

	sink.Close()

	if !droppedCalled.Load() {
		t.Error("OnDropped callback should have been called")
	}
}

func TestAsyncSink_Flush_DrainsQueue(t *testing.T) {
	inner := &slowSink{}
	sink := NewAsyncSink(inner, WithQueueSize(100))

	// Write several events
	for i := 0; i < 10; i++ {
		event := aisen.ErrorEvent{EventID: "evt-" + string(rune('0'+i))}
		sink.Write(context.Background(), event)
	}

	// Flush should wait for all events to be processed
	err := sink.Flush(context.Background())
	if err != nil {
		t.Fatalf("Flush returned error: %v", err)
	}

	// All events should have reached the inner sink
	events := inner.getEvents()
	if len(events) != 10 {
		t.Errorf("Expected 10 events after flush, got %d", len(events))
	}

	sink.Close()
}

func TestAsyncSink_Close_DrainsAndClosesInner(t *testing.T) {
	inner := &slowSink{}
	sink := NewAsyncSink(inner, WithQueueSize(100))

	// Write events
	for i := 0; i < 5; i++ {
		sink.Write(context.Background(), aisen.ErrorEvent{EventID: "evt"})
	}

	// Close should drain queue
	err := sink.Close()
	if err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	// Events should have been processed
	events := inner.getEvents()
	if len(events) != 5 {
		t.Errorf("Expected 5 events after close, got %d", len(events))
	}
}

func TestAsyncSink_DefaultQueueSize(t *testing.T) {
	inner := &slowSink{}
	sink := NewAsyncSink(inner) // No options - should use defaults
	defer sink.Close()

	// Should be able to write without panic
	err := sink.Write(context.Background(), aisen.ErrorEvent{})
	if err != nil {
		t.Errorf("Write with default options failed: %v", err)
	}
}

func TestAsyncSink_WriteAfterClose_ReturnsError(t *testing.T) {
	inner := &slowSink{}
	sink := NewAsyncSink(inner)
	sink.Close()

	err := sink.Write(context.Background(), aisen.ErrorEvent{})
	if err == nil {
		t.Error("Write after Close should return error")
	}
}
