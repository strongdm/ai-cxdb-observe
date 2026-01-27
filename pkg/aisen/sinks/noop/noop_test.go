package noop

import (
	"context"
	"testing"
	"time"

	"github.com/strongdm/ai-cxdb-observe/pkg/aisen"
)

func TestNoopSink_ImplementsSinkInterface(t *testing.T) {
	var _ aisen.Sink = NewNoopSink()
}

func TestNoopSink_Write_ReturnsNil(t *testing.T) {
	sink := NewNoopSink()

	event := aisen.ErrorEvent{
		EventID:   "evt-123",
		Timestamp: time.Now(),
		Severity:  aisen.SeverityError,
		ErrorType: "test",
		Message:   "test error",
	}

	err := sink.Write(context.Background(), event)
	if err != nil {
		t.Errorf("Write returned error: %v", err)
	}
}

func TestNoopSink_Flush_ReturnsNil(t *testing.T) {
	sink := NewNoopSink()

	err := sink.Flush(context.Background())
	if err != nil {
		t.Errorf("Flush returned error: %v", err)
	}
}

func TestNoopSink_Close_ReturnsNil(t *testing.T) {
	sink := NewNoopSink()

	err := sink.Close()
	if err != nil {
		t.Errorf("Close returned error: %v", err)
	}
}

func TestNoopSink_MultipleWrites(t *testing.T) {
	sink := NewNoopSink()

	for i := 0; i < 100; i++ {
		event := aisen.ErrorEvent{
			EventID: "evt-" + string(rune('0'+i%10)),
		}
		if err := sink.Write(context.Background(), event); err != nil {
			t.Fatalf("Write %d returned error: %v", i, err)
		}
	}
}
