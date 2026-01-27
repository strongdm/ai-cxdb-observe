// Package noop provides a no-operation sink that discards all events.
// Useful for testing and for disabling error collection.
package noop

import (
	"context"

	"github.com/strongdm/ai-cxdb-observe/pkg/aisen"
)

// noopSink discards all events.
type noopSink struct{}

// NewNoopSink creates a sink that discards all events.
// All methods return nil and perform no operations.
func NewNoopSink() aisen.Sink {
	return &noopSink{}
}

// Write discards the event and returns nil.
func (s *noopSink) Write(ctx context.Context, event aisen.ErrorEvent) error {
	return nil
}

// Flush is a no-op and returns nil.
func (s *noopSink) Flush(ctx context.Context) error {
	return nil
}

// Close is a no-op and returns nil.
func (s *noopSink) Close() error {
	return nil
}
