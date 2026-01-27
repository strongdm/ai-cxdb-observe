// collector.go provides the central Collector interface and default implementation.

package aisen

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Collector records error events to configured sinks.
type Collector interface {
	// Record captures an error event. Blocks until persisted (synchronous).
	// Applies scrubbing and fingerprinting before delegating to sinks.
	Record(ctx context.Context, event ErrorEvent) error

	// Flush ensures any buffered events are persisted.
	// For synchronous collectors, this may be a no-op.
	Flush(ctx context.Context) error

	// Close releases resources held by the collector.
	Close() error
}

// CollectorOption configures a Collector.
type CollectorOption func(*collectorConfig)

type collectorConfig struct {
	sink     Sink
	scrubber *Scrubber
}

// WithSink sets the sink for the collector.
func WithSink(sink Sink) CollectorOption {
	return func(c *collectorConfig) {
		c.sink = sink
	}
}

// WithScrubber configures the collector with a custom scrubber configuration.
func WithScrubber(cfg ScrubberConfig) CollectorOption {
	return func(c *collectorConfig) {
		c.scrubber = NewScrubber(cfg)
	}
}

// WithDefaultScrubbing enables scrubbing with production-safe defaults.
func WithDefaultScrubbing() CollectorOption {
	return func(c *collectorConfig) {
		c.scrubber = NewScrubber(DefaultScrubberConfig())
	}
}

// defaultCollector is the standard Collector implementation.
type defaultCollector struct {
	sink     Sink
	scrubber *Scrubber
}

// NewCollector creates a new Collector with the given options.
func NewCollector(opts ...CollectorOption) Collector {
	cfg := &collectorConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	// Default to a noop sink if none provided
	if cfg.sink == nil {
		cfg.sink = &noopSinkInternal{}
	}

	return &defaultCollector{
		sink:     cfg.sink,
		scrubber: cfg.scrubber,
	}
}

// Record captures an error event with scrubbing and fingerprinting.
func (c *defaultCollector) Record(ctx context.Context, event ErrorEvent) error {
	// Generate EventID if not set
	if event.EventID == "" {
		event.EventID = uuid.NewString()
	}

	// Set timestamp if not set
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// Apply scrubbing if configured
	if c.scrubber != nil {
		event.Message = c.scrubber.ScrubMessage(event.Message)
		event.StackTrace = c.scrubber.ScrubStackTrace(event.StackTrace)
		event.Metadata = c.scrubber.ScrubMetadata(event.Metadata)
		// Tool args scrubbing could be added here if needed
	}

	// Generate fingerprint
	event.Fingerprint = Fingerprint(event)

	// Write to sink
	return c.sink.Write(ctx, event)
}

// Flush delegates to the sink.
func (c *defaultCollector) Flush(ctx context.Context) error {
	return c.sink.Flush(ctx)
}

// Close delegates to the sink.
func (c *defaultCollector) Close() error {
	return c.sink.Close()
}

// noopSinkInternal is an internal noop sink to avoid import cycles.
type noopSinkInternal struct{}

func (s *noopSinkInternal) Write(ctx context.Context, event ErrorEvent) error {
	return nil
}

func (s *noopSinkInternal) Flush(ctx context.Context) error {
	return nil
}

func (s *noopSinkInternal) Close() error {
	return nil
}
