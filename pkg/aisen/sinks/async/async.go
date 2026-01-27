// Package async provides a sink wrapper with a bounded queue for high-throughput scenarios.
// Events are queued and processed asynchronously; oldest events are dropped when full.
package async

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/strongdm/ai-cxdb-observe/pkg/aisen"
)

// AsyncSinkOption configures the async sink.
type AsyncSinkOption func(*asyncSinkConfig)

type asyncSinkConfig struct {
	queueSize     int
	flushInterval time.Duration
	onDropped     func(count int)
}

// WithQueueSize sets the maximum number of queued events (default: 1000).
func WithQueueSize(size int) AsyncSinkOption {
	return func(c *asyncSinkConfig) {
		if size > 0 {
			c.queueSize = size
		}
	}
}

// WithFlushInterval sets how often to flush to the inner sink (default: 100ms).
func WithFlushInterval(d time.Duration) AsyncSinkOption {
	return func(c *asyncSinkConfig) {
		if d > 0 {
			c.flushInterval = d
		}
	}
}

// WithOnDropped sets a callback invoked when events are dropped due to queue overflow.
func WithOnDropped(fn func(count int)) AsyncSinkOption {
	return func(c *asyncSinkConfig) {
		c.onDropped = fn
	}
}

// asyncSink wraps a sink with a bounded queue.
type asyncSink struct {
	inner     aisen.Sink
	queue     chan aisen.ErrorEvent
	done      chan struct{}
	closeOnce sync.Once
	closeMu   sync.Mutex
	closed    bool
	wg        sync.WaitGroup
	onDropped func(count int)
}

// NewAsyncSink wraps a sink with a bounded queue for async writes.
// Write() returns immediately (<1ms); events are processed in the background.
// When the queue is full, the oldest event is dropped to make room.
func NewAsyncSink(inner aisen.Sink, opts ...AsyncSinkOption) aisen.Sink {
	cfg := &asyncSinkConfig{
		queueSize:     1000,
		flushInterval: 100 * time.Millisecond,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	s := &asyncSink{
		inner:     inner,
		queue:     make(chan aisen.ErrorEvent, cfg.queueSize),
		done:      make(chan struct{}),
		onDropped: cfg.onDropped,
	}

	s.wg.Add(1)
	go s.processLoop()

	return s
}

// processLoop drains the queue and writes to the inner sink.
func (s *asyncSink) processLoop() {
	defer s.wg.Done()
	for {
		select {
		case event, ok := <-s.queue:
			if !ok {
				return
			}
			// Ignore errors from inner sink (fire and forget)
			_ = s.inner.Write(context.Background(), event)
		case <-s.done:
			// Drain remaining events
			for {
				select {
				case event, ok := <-s.queue:
					if !ok {
						return
					}
					_ = s.inner.Write(context.Background(), event)
				default:
					return
				}
			}
		}
	}
}

// Write enqueues an event for async processing.
// Returns immediately. If the queue is full, drops the oldest event.
func (s *asyncSink) Write(ctx context.Context, event aisen.ErrorEvent) error {
	s.closeMu.Lock()
	if s.closed {
		s.closeMu.Unlock()
		return errors.New("async sink is closed")
	}
	s.closeMu.Unlock()

	// Try to enqueue
	select {
	case s.queue <- event:
		return nil
	default:
		// Queue is full - drop oldest and enqueue new
		s.dropOldestAndEnqueue(event)
		return nil
	}
}

// dropOldestAndEnqueue drops the oldest event and enqueues the new one.
func (s *asyncSink) dropOldestAndEnqueue(event aisen.ErrorEvent) {
	// Try to read (drop) one event from the queue
	select {
	case <-s.queue:
		if s.onDropped != nil {
			s.onDropped(1)
		}
	default:
		// Queue was emptied by processor, try again
	}

	// Now try to enqueue again
	select {
	case s.queue <- event:
	default:
		// Still full, just drop the new event
		if s.onDropped != nil {
			s.onDropped(1)
		}
	}
}

// Flush blocks until all queued events are processed.
func (s *asyncSink) Flush(ctx context.Context) error {
	// Wait for queue to drain by checking periodically
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if len(s.queue) == 0 {
				// Give a moment for the last event to be processed
				time.Sleep(10 * time.Millisecond)
				return s.inner.Flush(ctx)
			}
		}
	}
}

// Close stops the async processor and closes the inner sink.
func (s *asyncSink) Close() error {
	s.closeOnce.Do(func() {
		s.closeMu.Lock()
		s.closed = true
		s.closeMu.Unlock()

		// Signal done and wait for drain
		close(s.done)
		s.wg.Wait()
		close(s.queue)
	})

	return s.inner.Close()
}
