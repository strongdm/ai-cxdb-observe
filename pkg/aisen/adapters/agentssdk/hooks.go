// hooks.go implements RunHooks for capturing operation context for enrichment.
// This adapter provides ENRICHMENT only - error detection is done by RunWrapper.

package agentssdk

import (
	"context"
	"log"
	"time"

	"github.com/strongdm/ai-agents-sdk/pkg/agents"
	llmsdk "github.com/strongdm/ai-llm-sdk/pkg/llm"
	"github.com/strongdm/ai-cxdb-observe/pkg/aisen"
)

// HookAdapter implements agents.RunHooks to capture operation context.
// It delegates to an inner RunHooks and captures enrichment data for correlation.
type HookAdapter struct {
	store  EnrichmentStore
	inner  agents.RunHooks
	logger *log.Logger
}

// NewHookAdapter wraps an existing RunHooks and captures operation context.
// This adapter provides ENRICHMENT only - error detection is done by RunWrapper.
//
// The store is used to correlate hook data with errors captured at the runner boundary.
// The inner hooks (if non-nil) are called for all hook methods; only their errors are returned.
// The logger is used for debug output (can be nil for no logging).
func NewHookAdapter(store EnrichmentStore, inner agents.RunHooks, logger *log.Logger) agents.RunHooks {
	return &HookAdapter{
		store:  store,
		inner:  inner,
		logger: logger,
	}
}

// OnAgentStart captures the agent name for enrichment.
func (h *HookAdapter) OnAgentStart(ctx context.Context, runCtx *agents.AgentHookContext, agent *agents.Agent) error {
	h.captureAgentInfo(ctx, agent)

	if h.inner != nil {
		return h.inner.OnAgentStart(ctx, runCtx, agent)
	}
	return nil
}

// OnAgentEnd delegates to inner hooks.
func (h *HookAdapter) OnAgentEnd(ctx context.Context, runCtx *agents.AgentHookContext, agent *agents.Agent, result agents.RunResult) error {
	if h.inner != nil {
		return h.inner.OnAgentEnd(ctx, runCtx, agent, result)
	}
	return nil
}

// OnHandoff delegates to inner hooks.
func (h *HookAdapter) OnHandoff(ctx context.Context, runCtx *agents.RunContext, from *agents.Agent, to *agents.Agent) error {
	if h.inner != nil {
		return h.inner.OnHandoff(ctx, runCtx, from, to)
	}
	return nil
}

// OnToolStart captures tool context for enrichment and records operation history.
func (h *HookAdapter) OnToolStart(ctx context.Context, runCtx *agents.RunContext, agent *agents.Agent, tool agents.Tool, call llmsdk.ToolCall) error {
	if runID, ok := aisen.RunIDFromContext(ctx); ok {
		h.store.Update(runID, func(e *Enrichment) {
			agentName := ""
			if agent != nil {
				e.AgentName = agent.Name()
				agentName = agent.Name()
			}
			e.Operation = "tool"
			e.ToolName = tool.Name
			e.ToolCallID = call.ID
			e.OperationID = call.ID

			// Record operation in history
			input := string(call.Arguments)
			record := OperationRecord{
				Kind:      "tool",
				Timestamp: time.Now(),
				AgentName: agentName,
				Tool: &ToolOperation{
					Name:      call.Name,
					CallID:    call.ID,
					Input:     input,
					InputSize: len(input),
				},
			}
			e.RecordOperation(record)
		})
	}

	if h.inner != nil {
		return h.inner.OnToolStart(ctx, runCtx, agent, tool, call)
	}
	return nil
}

// OnToolEnd updates the last operation with output data and delegates to inner hooks.
func (h *HookAdapter) OnToolEnd(ctx context.Context, runCtx *agents.RunContext, agent *agents.Agent, tool agents.Tool, output string) error {
	if runID, ok := aisen.RunIDFromContext(ctx); ok {
		h.store.Update(runID, func(e *Enrichment) {
			// Update the last operation in history with output data
			if e.operationHistory != nil {
				endTime := time.Now()
				e.operationHistory.UpdateLast(func(rec *OperationRecord) {
					if rec.Kind == "tool" && rec.Tool != nil {
						// Update with output
						rec.Tool.Output = output
						rec.Tool.OutputSize = len(output)
						// Calculate duration
						rec.Duration = endTime.Sub(rec.Timestamp).Milliseconds()
					}
				})
			}
		})
	}

	if h.inner != nil {
		return h.inner.OnToolEnd(ctx, runCtx, agent, tool, output)
	}
	return nil
}

// OnLLMStart captures LLM context for enrichment and records operation history.
func (h *HookAdapter) OnLLMStart(ctx context.Context, runCtx *agents.RunContext, agent *agents.Agent, req llmsdk.Request) error {
	if runID, ok := aisen.RunIDFromContext(ctx); ok {
		h.store.Update(runID, func(e *Enrichment) {
			agentName := ""
			if agent != nil {
				e.AgentName = agent.Name()
				agentName = agent.Name()
			}
			e.Operation = "llm"
			e.Model = req.Model

			// Record operation in history
			record := OperationRecord{
				Kind:      "llm",
				Timestamp: time.Now(),
				AgentName: agentName,
				LLM:       buildLLMOperation(req),
			}
			e.RecordOperation(record)
		})
	}

	if h.inner != nil {
		return h.inner.OnLLMStart(ctx, runCtx, agent, req)
	}
	return nil
}

// OnLLMEnd updates the last operation with response metadata and delegates to inner hooks.
func (h *HookAdapter) OnLLMEnd(ctx context.Context, runCtx *agents.RunContext, agent *agents.Agent, resp llmsdk.Response) error {
	if runID, ok := aisen.RunIDFromContext(ctx); ok {
		h.store.Update(runID, func(e *Enrichment) {
			// Update the last operation in history with response data
			if e.operationHistory != nil {
				endTime := time.Now()
				e.operationHistory.UpdateLast(func(rec *OperationRecord) {
					if rec.Kind == "llm" && rec.LLM != nil {
						// Update with response metadata
						updateLLMOperationWithResponse(rec.LLM, resp)
						// Calculate duration
						rec.Duration = endTime.Sub(rec.Timestamp).Milliseconds()
					}
				})
			}
		})
	}

	if h.inner != nil {
		return h.inner.OnLLMEnd(ctx, runCtx, agent, resp)
	}
	return nil
}

// captureAgentInfo captures agent information when available.
func (h *HookAdapter) captureAgentInfo(ctx context.Context, agent *agents.Agent) {
	if agent == nil {
		return
	}
	if runID, ok := aisen.RunIDFromContext(ctx); ok {
		h.store.Update(runID, func(e *Enrichment) {
			e.AgentName = agent.Name()
		})
	}
}
