// sink.go defines the Sink interface for error event destinations.

package aisen

import "context"

// Sink is the destination for error events.
// Implementations must be safe for concurrent use.
type Sink interface {
	// Write persists an error event. Called after scrubbing/enrichment.
	// Implementations should be idempotent when possible.
	Write(ctx context.Context, event ErrorEvent) error

	// Flush ensures any buffered events are persisted.
	// For synchronous sinks, this may be a no-op.
	Flush(ctx context.Context) error

	// Close releases resources held by the sink.
	// After Close is called, Write and Flush should return errors.
	Close() error
}
