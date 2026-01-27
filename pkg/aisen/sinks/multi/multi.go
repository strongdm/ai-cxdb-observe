// Package multi provides a sink that fans out to multiple sinks.
// All sinks receive all events; errors are aggregated.
package multi

import (
	"context"
	"errors"

	"github.com/strongdm/ai-cxdb-observe/pkg/aisen"
)

// multiSink fans out to multiple sinks.
type multiSink struct {
	sinks []aisen.Sink
}

// NewMultiSink creates a sink that writes to multiple sinks.
// All sinks receive all events. Errors are aggregated via errors.Join.
func NewMultiSink(sinks ...aisen.Sink) aisen.Sink {
	return &multiSink{
		sinks: sinks,
	}
}

// Write sends the event to all sinks, collecting any errors.
// All sinks are called even if some return errors.
func (s *multiSink) Write(ctx context.Context, event aisen.ErrorEvent) error {
	var errs []error
	for _, sink := range s.sinks {
		if err := sink.Write(ctx, event); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Flush calls Flush on all sinks, collecting any errors.
func (s *multiSink) Flush(ctx context.Context) error {
	var errs []error
	for _, sink := range s.sinks {
		if err := sink.Flush(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Close calls Close on all sinks, collecting any errors.
func (s *multiSink) Close() error {
	var errs []error
	for _, sink := range s.sinks {
		if err := sink.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
