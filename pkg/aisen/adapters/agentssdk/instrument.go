// instrument.go provides the Instrument function for convenient runner setup.
// This is the recommended entry point for integrating aisen with ai-agents-sdk.

package agentssdk

import (
	"log"
	"time"

	"github.com/strongdm/ai-agents-sdk/pkg/agents"
	"github.com/strongdm/ai-cxdb-observe/pkg/aisen"
)

// WrapOption configures a WrappedRunner.
type WrapOption func(*WrappedRunner)

// WithLogger sets the logger for the wrapper.
// The logger is used for debug output when recording errors fails.
func WithLogger(logger *log.Logger) WrapOption {
	return func(w *WrappedRunner) {
		w.logger = logger
	}
}

// WithEnrichmentStore sets the enrichment store for the wrapper.
// The store is used to correlate hook data with errors captured at the runner boundary.
func WithEnrichmentStore(store EnrichmentStore) WrapOption {
	return func(w *WrappedRunner) {
		w.enrichments = store
	}
}

// Instrument wraps a Runner with error and panic capture.
// This is the recommended entry point for integrating aisen with ai-agents-sdk.
//
// Example:
//
//	collector := aisen.NewCollector(sink)
//	runner := agents.NewRunner(client)
//	wrapped := agentssdk.Instrument(runner, collector)
//	result, err := wrapped.Run(ctx, agent, input, session, nil)
func Instrument(baseRunner *agents.Runner, collector aisen.Collector, opts ...WrapOption) *WrappedRunner {
	wrapper := &WrappedRunner{
		inner:       baseRunner,
		collector:   collector,
		enrichments: NewEnrichmentStore(),
		startTime:   time.Now(),
	}

	for _, opt := range opts {
		opt(wrapper)
	}

	return wrapper
}
