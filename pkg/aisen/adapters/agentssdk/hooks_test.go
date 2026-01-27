package agentssdk

import (
	"context"
	"errors"
	"log"
	"os"
	"testing"

	"github.com/strongdm/ai-agents-sdk/pkg/agents"
	llmsdk "github.com/strongdm/ai-llm-sdk/pkg/llm"
	"github.com/strongdm/ai-cxdb-observe/pkg/aisen"
)

// mockRunHooks implements agents.RunHooks for testing.
type mockRunHooks struct {
	agentStartCalled bool
	toolStartCalled  bool
	llmStartCalled   bool
	returnErr        error
}

func (m *mockRunHooks) OnAgentStart(ctx context.Context, runCtx *agents.AgentHookContext, agent *agents.Agent) error {
	m.agentStartCalled = true
	return m.returnErr
}

func (m *mockRunHooks) OnAgentEnd(ctx context.Context, runCtx *agents.AgentHookContext, agent *agents.Agent, result agents.RunResult) error {
	return m.returnErr
}

func (m *mockRunHooks) OnHandoff(ctx context.Context, runCtx *agents.RunContext, from *agents.Agent, to *agents.Agent) error {
	return m.returnErr
}

func (m *mockRunHooks) OnToolStart(ctx context.Context, runCtx *agents.RunContext, agent *agents.Agent, tool agents.Tool, call llmsdk.ToolCall) error {
	m.toolStartCalled = true
	return m.returnErr
}

func (m *mockRunHooks) OnToolEnd(ctx context.Context, runCtx *agents.RunContext, agent *agents.Agent, tool agents.Tool, output string) error {
	return m.returnErr
}

func (m *mockRunHooks) OnLLMStart(ctx context.Context, runCtx *agents.RunContext, agent *agents.Agent, req llmsdk.Request) error {
	m.llmStartCalled = true
	return m.returnErr
}

func (m *mockRunHooks) OnLLMEnd(ctx context.Context, runCtx *agents.RunContext, agent *agents.Agent, resp llmsdk.Response) error {
	return m.returnErr
}

func TestHookAdapter_ImplementsRunHooks(t *testing.T) {
	store := NewEnrichmentStore()
	logger := log.New(os.Stderr, "", 0)
	var _ agents.RunHooks = NewHookAdapter(store, nil, logger)
}

func TestHookAdapter_OnToolStart_CapturesEnrichment(t *testing.T) {
	store := NewEnrichmentStore()
	logger := log.New(os.Stderr, "", 0)
	adapter := NewHookAdapter(store, nil, logger)

	ctx := context.Background()
	ctx = aisen.WithRunID(ctx, "run-123")

	agent := agents.NewAgent(agents.AgentConfig{Name: "test-agent"})
	tool := agents.Tool{Name: "WebSearch"}
	call := llmsdk.ToolCall{ID: "call-456"}

	err := adapter.OnToolStart(ctx, nil, agent, tool, call)
	if err != nil {
		t.Fatalf("OnToolStart returned error: %v", err)
	}

	// Check enrichment was captured
	enrichment, ok := store.Get("run-123")
	if !ok {
		t.Fatal("Enrichment not found for run-123")
	}

	if enrichment.AgentName != "test-agent" {
		t.Errorf("AgentName = %q, want %q", enrichment.AgentName, "test-agent")
	}
	if enrichment.ToolName != "WebSearch" {
		t.Errorf("ToolName = %q, want %q", enrichment.ToolName, "WebSearch")
	}
	if enrichment.ToolCallID != "call-456" {
		t.Errorf("ToolCallID = %q, want %q", enrichment.ToolCallID, "call-456")
	}
	if enrichment.Operation != "tool" {
		t.Errorf("Operation = %q, want %q", enrichment.Operation, "tool")
	}
}

func TestHookAdapter_OnAgentStart_CapturesAgentName(t *testing.T) {
	store := NewEnrichmentStore()
	logger := log.New(os.Stderr, "", 0)
	adapter := NewHookAdapter(store, nil, logger)

	ctx := context.Background()
	ctx = aisen.WithRunID(ctx, "run-456")

	agent := agents.NewAgent(agents.AgentConfig{Name: "my-agent"})

	err := adapter.OnAgentStart(ctx, nil, agent)
	if err != nil {
		t.Fatalf("OnAgentStart returned error: %v", err)
	}

	enrichment, ok := store.Get("run-456")
	if !ok {
		t.Fatal("Enrichment not found")
	}

	if enrichment.AgentName != "my-agent" {
		t.Errorf("AgentName = %q, want %q", enrichment.AgentName, "my-agent")
	}
}

func TestHookAdapter_DelegatesToInner(t *testing.T) {
	store := NewEnrichmentStore()
	logger := log.New(os.Stderr, "", 0)
	inner := &mockRunHooks{}
	adapter := NewHookAdapter(store, inner, logger)

	ctx := context.Background()
	ctx = aisen.WithRunID(ctx, "run-test")

	agent := agents.NewAgent(agents.AgentConfig{Name: "agent"})
	tool := agents.Tool{Name: "Tool"}

	// Call OnToolStart
	adapter.OnToolStart(ctx, nil, agent, tool, llmsdk.ToolCall{})

	if !inner.toolStartCalled {
		t.Error("Inner hook OnToolStart was not called")
	}
}

func TestHookAdapter_ReturnsInnerError(t *testing.T) {
	store := NewEnrichmentStore()
	logger := log.New(os.Stderr, "", 0)
	expectedErr := errors.New("inner hook error")
	inner := &mockRunHooks{returnErr: expectedErr}
	adapter := NewHookAdapter(store, inner, logger)

	ctx := context.Background()
	ctx = aisen.WithRunID(ctx, "run-test")

	agent := agents.NewAgent(agents.AgentConfig{Name: "agent"})
	tool := agents.Tool{Name: "Tool"}

	err := adapter.OnToolStart(ctx, nil, agent, tool, llmsdk.ToolCall{})

	if !errors.Is(err, expectedErr) {
		t.Errorf("Expected inner error %v, got %v", expectedErr, err)
	}
}

func TestHookAdapter_HandlesNoRunID(t *testing.T) {
	store := NewEnrichmentStore()
	logger := log.New(os.Stderr, "", 0)
	adapter := NewHookAdapter(store, nil, logger)

	ctx := context.Background() // No run ID

	agent := agents.NewAgent(agents.AgentConfig{Name: "agent"})

	// Should not panic
	err := adapter.OnAgentStart(ctx, nil, agent)
	if err != nil {
		t.Errorf("OnAgentStart returned error: %v", err)
	}
}

func TestHookAdapter_OnLLMStart_CapturesModel(t *testing.T) {
	store := NewEnrichmentStore()
	logger := log.New(os.Stderr, "", 0)
	adapter := NewHookAdapter(store, nil, logger)

	ctx := context.Background()
	ctx = aisen.WithRunID(ctx, "run-llm")

	agent := agents.NewAgent(agents.AgentConfig{Name: "agent"})
	req := llmsdk.Request{Model: "gpt-4o"}

	err := adapter.OnLLMStart(ctx, nil, agent, req)
	if err != nil {
		t.Fatalf("OnLLMStart returned error: %v", err)
	}

	enrichment, ok := store.Get("run-llm")
	if !ok {
		t.Fatal("Enrichment not found")
	}

	if enrichment.Model != "gpt-4o" {
		t.Errorf("Model = %q, want %q", enrichment.Model, "gpt-4o")
	}
	if enrichment.Operation != "llm" {
		t.Errorf("Operation = %q, want %q", enrichment.Operation, "llm")
	}
}
