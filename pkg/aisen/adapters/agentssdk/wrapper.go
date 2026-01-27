// wrapper.go implements WrappedRunner that wraps agents.Runner to capture errors and panics.
// This is the PRIMARY error capture mechanism - hooks provide enrichment only.

package agentssdk

import (
	"context"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/strongdm/ai-agents-sdk/pkg/agents"
	"github.com/strongdm/ai-cxdb-observe/pkg/aisen"
)

// WrappedRunner wraps an agents.Runner to capture errors and panics.
// It is the primary error capture mechanism - hooks provide enrichment only.
type WrappedRunner struct {
	inner       *agents.Runner
	collector   aisen.Collector
	enrichments EnrichmentStore
	logger      *log.Logger
	startTime   time.Time
}

// NewWrappedRunner creates a new WrappedRunner that wraps the given Runner.
// The collector is used to record errors and panics.
// The enrichment store is used to correlate hook data with errors.
// The logger is used for debug output (can be nil for no logging).
func NewWrappedRunner(inner *agents.Runner, collector aisen.Collector, store EnrichmentStore, logger *log.Logger) *WrappedRunner {
	if store == nil {
		store = NewEnrichmentStore()
	}
	return &WrappedRunner{
		inner:       inner,
		collector:   collector,
		enrichments: store,
		logger:      logger,
		startTime:   time.Now(),
	}
}

// Run executes the agent with the given input and session, capturing any errors or panics.
func (w *WrappedRunner) Run(ctx context.Context, agent *agents.Agent, input string, session agents.Session, cfg *agents.RunConfig) (agents.RunResult, error) {
	runID := uuid.New().String()
	ctx = aisen.WithRunID(ctx, runID)
	defer w.enrichments.Delete(runID)

	// Extract context ID from session if it implements ContextIDProvider
	contextID := w.extractContextID(ctx, session)

	// Wrap hooks to capture enrichment while preserving user-provided hooks.
	wrappedCfg := w.wrapRunConfig(cfg)

	// Capture panics
	defer w.capturePanic(ctx, runID, contextID)

	result, err := w.inner.Run(ctx, agent, input, session, wrappedCfg)
	if err != nil {
		w.captureError(ctx, runID, contextID, err)
	}
	return result, err
}

// RunOnce executes a single turn of the agent, capturing any errors or panics.
func (w *WrappedRunner) RunOnce(ctx context.Context, agent *agents.Agent, input string, cfg *agents.RunConfig) (agents.RunResult, error) {
	runID := uuid.New().String()
	ctx = aisen.WithRunID(ctx, runID)
	defer w.enrichments.Delete(runID)

	// No session in RunOnce, so no context ID extraction
	var contextID uint64

	// Wrap hooks to capture enrichment while preserving user-provided hooks.
	wrappedCfg := w.wrapRunConfig(cfg)

	// Capture panics
	defer w.capturePanic(ctx, runID, contextID)

	result, err := w.inner.RunOnce(ctx, agent, input, wrappedCfg)
	if err != nil {
		w.captureError(ctx, runID, contextID, err)
	}
	return result, err
}

// RunStream starts a streaming run, capturing any errors at the start.
// Note: Errors during streaming are not captured by this wrapper.
func (w *WrappedRunner) RunStream(ctx context.Context, agent *agents.Agent, input string, session agents.Session, cfg *agents.RunConfig) (*agents.StreamingRun, error) {
	runID := uuid.New().String()
	ctx = aisen.WithRunID(ctx, runID)
	// Note: We don't defer Delete here because the stream may outlive this call.
	// The enrichment will be cleaned up when the context is cancelled.

	// Extract context ID from session if it implements ContextIDProvider
	contextID := w.extractContextID(ctx, session)

	// Wrap hooks to capture enrichment while preserving user-provided hooks.
	wrappedCfg := w.wrapRunConfig(cfg)

	// Capture panics
	defer w.capturePanic(ctx, runID, contextID)

	stream, err := w.inner.RunStream(ctx, agent, input, session, wrappedCfg)
	if err != nil {
		w.captureError(ctx, runID, contextID, err)
		w.enrichments.Delete(runID)
	}
	return stream, err
}

// extractContextID extracts the context ID from a session if it implements ContextIDProvider.
func (w *WrappedRunner) extractContextID(ctx context.Context, session interface{}) uint64 {
	if provider, ok := session.(aisen.ContextIDProvider); ok {
		if id, err := provider.ContextID(ctx); err == nil {
			return id
		}
	}
	// Fallback to context propagation when session cannot provide a context ID.
	if id, ok := aisen.ContextIDFromContext(ctx); ok {
		return id
	}
	return 0
}

// wrapRunConfig clones cfg and wraps hooks with HookAdapter for enrichment capture.
func (w *WrappedRunner) wrapRunConfig(cfg *agents.RunConfig) *agents.RunConfig {
	var cloned agents.RunConfig
	if cfg != nil {
		cloned = *cfg
	}
	cloned.Hooks = NewHookAdapter(w.enrichments, cloned.Hooks, w.logger)
	return &cloned
}

// captureError records an error event with enrichment data.
func (w *WrappedRunner) captureError(ctx context.Context, runID string, contextID uint64, err error) {
	enrichment, _ := w.enrichments.Get(runID)
	event := buildErrorEvent(err, contextID, enrichment)
	event.SystemState = aisen.CaptureSystemState(w.startTime)
	w.safeRecord(ctx, event)
}

// capturePanic recovers from a panic, records it, and re-panics.
func (w *WrappedRunner) capturePanic(ctx context.Context, runID string, contextID uint64) {
	if r := recover(); r != nil {
		enrichment, _ := w.enrichments.Get(runID)
		event := buildPanicEvent(r, contextID, enrichment)
		event.SystemState = aisen.CaptureSystemState(w.startTime)
		w.safeRecord(ctx, event)
		panic(r)
	}
}

// safeRecord records an event, logging any errors rather than propagating them.
func (w *WrappedRunner) safeRecord(ctx context.Context, event aisen.ErrorEvent) {
	if err := w.collector.Record(ctx, event); err != nil && w.logger != nil {
		w.logger.Printf("aisen: failed to record error: %v", err)
	}
}

// Inner returns the underlying Runner for advanced usage.
func (w *WrappedRunner) Inner() *agents.Runner {
	return w.inner
}
