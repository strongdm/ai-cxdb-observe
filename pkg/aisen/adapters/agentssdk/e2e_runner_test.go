package agentssdk

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/strongdm/ai-agents-sdk/pkg/agents"
	"github.com/strongdm/ai-cxdb-observe/pkg/aisen"
	llmsdk "github.com/strongdm/ai-llm-sdk/pkg/llm"
	llmmock "github.com/strongdm/ai-llm-sdk/pkg/llm/mock"
)

func newMockClient(adapter *llmmock.Adapter) *llmsdk.Client {
	return llmsdk.NewClient(
		map[llmsdk.Provider]llmsdk.ProviderAdapter{llmsdk.ProviderOpenAI: adapter},
		llmsdk.WithDefaultProvider(llmsdk.ProviderOpenAI),
	)
}

type spyHooks struct {
	mu    sync.Mutex
	calls map[string]int
}

func newSpyHooks() *spyHooks {
	return &spyHooks{calls: map[string]int{}}
}

func (h *spyHooks) record(name string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls[name]++
}

func (h *spyHooks) count(name string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.calls[name]
}

func (h *spyHooks) OnAgentStart(ctx context.Context, runCtx *agents.AgentHookContext, agent *agents.Agent) error {
	h.record("agent_start")
	return nil
}

func (h *spyHooks) OnAgentEnd(ctx context.Context, runCtx *agents.AgentHookContext, agent *agents.Agent, result agents.RunResult) error {
	h.record("agent_end")
	return nil
}

func (h *spyHooks) OnHandoff(ctx context.Context, runCtx *agents.RunContext, from *agents.Agent, to *agents.Agent) error {
	h.record("handoff")
	return nil
}

func (h *spyHooks) OnToolStart(ctx context.Context, runCtx *agents.RunContext, agent *agents.Agent, tool agents.Tool, call llmsdk.ToolCall) error {
	h.record("tool_start")
	return nil
}

func (h *spyHooks) OnToolEnd(ctx context.Context, runCtx *agents.RunContext, agent *agents.Agent, tool agents.Tool, output string) error {
	h.record("tool_end")
	return nil
}

func (h *spyHooks) OnLLMStart(ctx context.Context, runCtx *agents.RunContext, agent *agents.Agent, req llmsdk.Request) error {
	h.record("llm_start")
	return nil
}

func (h *spyHooks) OnLLMEnd(ctx context.Context, runCtx *agents.RunContext, agent *agents.Agent, resp llmsdk.Response) error {
	h.record("llm_end")
	return nil
}

func enqueueToolCall(adapter *llmmock.Adapter, toolName, callID string) {
	call := llmsdk.ToolCall{
		ID:        callID,
		Name:      toolName,
		Arguments: json.RawMessage(`{"query":"hi"}`),
	}
	resp := llmsdk.Response{
		Model:        "test-model",
		Message:      llmsdk.Message{Role: llmsdk.RoleAssistant},
		ToolCalls:    []llmsdk.ToolCall{call},
		FinishReason: llmsdk.FinishReasonToolCalls,
	}
	adapter.EnqueueComplete(resp, nil)
}

func TestE2E_RunWrapper_CorrelatesHookEnrichment(t *testing.T) {
	adapter := &llmmock.Adapter{}
	enqueueToolCall(adapter, "FailTool", "call-1")

	collectorSink := &capturingSink{}
	collector := aisen.NewCollector(
		aisen.WithSink(collectorSink),
		aisen.WithDefaultScrubbing(),
	)

	runner := agents.NewRunner(newMockClient(adapter))
	wrapped := Instrument(runner, collector)

	tool := agents.Tool{
		Name: "FailTool",
		Handler: func(ctx context.Context, args json.RawMessage) (string, error) {
			return "", errors.New("tool execution failed")
		},
	}

	agent := agents.NewAgent(agents.AgentConfig{
		Name:         "e2e-agent",
		Instructions: "be helpful",
		Model:        "test-model",
		Tools:        []agents.Tool{tool},
	})

	spy := newSpyHooks()
	cfg := &agents.RunConfig{Hooks: spy, MaxTurns: 2}

	_, err := wrapped.Run(context.Background(), agent, "trigger tool", nil, cfg)
	if err == nil {
		t.Fatalf("expected error from failing tool")
	}

	events := collectorSink.getEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 captured event, got %d", len(events))
	}

	event := events[0]
	if event.AgentName != "e2e-agent" {
		t.Errorf("AgentName = %q, want %q", event.AgentName, "e2e-agent")
	}
	if event.Operation != "tool" {
		t.Errorf("Operation = %q, want %q", event.Operation, "tool")
	}
	if event.ToolName != "FailTool" {
		t.Errorf("ToolName = %q, want %q", event.ToolName, "FailTool")
	}
	if event.OperationID != "call-1" {
		t.Errorf("OperationID = %q, want %q", event.OperationID, "call-1")
	}

	if spy.count("agent_start") == 0 {
		t.Errorf("expected inner hooks to run: agent_start not called")
	}
	if spy.count("llm_start") == 0 {
		t.Errorf("expected inner hooks to run: llm_start not called")
	}
	if spy.count("tool_start") == 0 {
		t.Errorf("expected inner hooks to run: tool_start not called")
	}
}

func TestE2E_RunWrapper_ContextIDFromContextFallback(t *testing.T) {
	adapter := &llmmock.Adapter{}
	adapter.EnqueueComplete(llmsdk.Response{}, errors.New("llm failed"))

	collectorSink := &capturingSink{}
	collector := aisen.NewCollector(aisen.WithSink(collectorSink))

	runner := agents.NewRunner(newMockClient(adapter))
	wrapped := Instrument(runner, collector)

	agent := agents.NewAgent(agents.AgentConfig{
		Name:         "context-agent",
		Instructions: "be helpful",
		Model:        "test-model",
	})

	contextID := uint64(424242)
	ctx := aisen.WithContextID(context.Background(), contextID)

	_, err := wrapped.Run(ctx, agent, "hi", nil, nil)
	if err == nil {
		t.Fatalf("expected llm error")
	}

	events := collectorSink.getEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 captured event, got %d", len(events))
	}
	if events[0].ContextID == nil || *events[0].ContextID != contextID {
		t.Fatalf("ContextID = %v, want %d", events[0].ContextID, contextID)
	}
}

func TestE2E_RunWrapper_CapturesSystemState(t *testing.T) {
	t.Run("error path", func(t *testing.T) {
		adapter := &llmmock.Adapter{}
		adapter.EnqueueComplete(llmsdk.Response{}, errors.New("llm failed"))

		collectorSink := &capturingSink{}
		collector := aisen.NewCollector(aisen.WithSink(collectorSink))

		runner := agents.NewRunner(newMockClient(adapter))
		wrapped := Instrument(runner, collector)

		agent := agents.NewAgent(agents.AgentConfig{
			Name:         "system-state-agent",
			Instructions: "be helpful",
			Model:        "test-model",
		})

		_, err := wrapped.Run(context.Background(), agent, "hi", nil, nil)
		if err == nil {
			t.Fatalf("expected llm error")
		}

		events := collectorSink.getEvents()
		if len(events) != 1 {
			t.Fatalf("expected 1 captured event, got %d", len(events))
		}

		state := events[0].SystemState
		if state == nil {
			t.Fatalf("SystemState should be captured")
		}
		if state.GoroutineCount <= 0 {
			t.Errorf("GoroutineCount = %d, want > 0", state.GoroutineCount)
		}
		if state.UptimeMs < 0 {
			t.Errorf("UptimeMs = %d, want >= 0", state.UptimeMs)
		}
		if state.MemoryBytes < 0 {
			t.Errorf("MemoryBytes = %d, want >= 0", state.MemoryBytes)
		}
	})

	t.Run("panic path", func(t *testing.T) {
		adapter := &llmmock.Adapter{}
		enqueueToolCall(adapter, "PanicTool", "call-2")

		collectorSink := &capturingSink{}
		collector := aisen.NewCollector(aisen.WithSink(collectorSink))

		runner := agents.NewRunner(newMockClient(adapter))
		wrapped := Instrument(runner, collector)

		tool := agents.Tool{
			Name: "PanicTool",
			Handler: func(ctx context.Context, args json.RawMessage) (string, error) {
				panic("tool panicked")
			},
		}

		agent := agents.NewAgent(agents.AgentConfig{
			Name:         "panic-agent",
			Instructions: "be helpful",
			Model:        "test-model",
			Tools:        []agents.Tool{tool},
		})

		defer func() {
			r := recover()
			if r == nil {
				t.Fatalf("expected panic to be re-raised")
			}

			events := collectorSink.getEvents()
			if len(events) != 1 {
				t.Fatalf("expected 1 captured event, got %d", len(events))
			}
			state := events[0].SystemState
			if state == nil {
				t.Fatalf("SystemState should be captured on panic")
			}
			if state.GoroutineCount <= 0 {
				t.Errorf("GoroutineCount = %d, want > 0", state.GoroutineCount)
			}
		}()

		wrapped.Run(context.Background(), agent, "trigger panic", nil, nil)
	})
}
