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

// TestHookAdapterCapturesLLMMetadata verifies request metadata is captured in operation history.
func TestHookAdapterCapturesLLMMetadata(t *testing.T) {
	store := NewEnrichmentStore()
	adapter := NewHookAdapter(store, nil, nil)

	ctx := aisen.WithRunID(context.Background(), "run-1")
	agent := agents.NewAgent(agents.AgentConfig{Name: "test-agent"})

	temp := float32(0.7)
	maxTokens := 100
	req := llmsdk.Request{
		Model:       "claude-3",
		Provider:    "anthropic",
		Messages:    []llmsdk.Message{{Role: "user", Parts: []llmsdk.ContentPart{{Text: "Hello"}}}},
		Temperature: &temp,
		MaxTokens:   &maxTokens,
	}

	err := adapter.OnLLMStart(ctx, nil, agent, req)
	if err != nil {
		t.Fatalf("OnLLMStart failed: %v", err)
	}

	enrichment, _ := store.Get("run-1")
	history := enrichment.GetOperationHistory()

	if len(history) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(history))
	}

	op := history[0]
	if op.Kind != "llm" {
		t.Errorf("Kind = %q, want %q", op.Kind, "llm")
	}
	if op.AgentName != "test-agent" {
		t.Errorf("AgentName = %q, want %q", op.AgentName, "test-agent")
	}
	if op.LLM == nil {
		t.Fatal("LLM field is nil")
	}
	if op.LLM.Model != "claude-3" {
		t.Errorf("Model = %q, want %q", op.LLM.Model, "claude-3")
	}
	if op.LLM.Provider != "anthropic" {
		t.Errorf("Provider = %q, want %q", op.LLM.Provider, "anthropic")
	}
	if op.LLM.MessageCount != 1 {
		t.Errorf("MessageCount = %d, want 1", op.LLM.MessageCount)
	}
	if op.LLM.Temperature == nil || *op.LLM.Temperature != 0.7 {
		t.Errorf("Temperature not captured correctly")
	}
	if op.LLM.MaxTokens == nil || *op.LLM.MaxTokens != 100 {
		t.Errorf("MaxTokens not captured correctly")
	}
}

// TestHookAdapterUpdatesLLMResponse verifies response metadata updates existing record.
func TestHookAdapterUpdatesLLMResponse(t *testing.T) {
	store := NewEnrichmentStore()
	adapter := NewHookAdapter(store, nil, nil)

	ctx := aisen.WithRunID(context.Background(), "run-2")
	agent := agents.NewAgent(agents.AgentConfig{Name: "agent"})

	req := llmsdk.Request{
		Model:    "gpt-4",
		Provider: "openai",
		Messages: []llmsdk.Message{{Role: "user", Parts: []llmsdk.ContentPart{{Text: "Test"}}}},
	}

	// Start LLM call
	adapter.OnLLMStart(ctx, nil, agent, req)

	// End LLM call with response
	resp := llmsdk.Response{
		ID:           "resp-123",
		FinishReason: "stop",
		Usage: llmsdk.Usage{
			PromptTokens:     50,
			CompletionTokens: 25,
			TotalTokens:      75,
		},
	}

	err := adapter.OnLLMEnd(ctx, nil, agent, resp)
	if err != nil {
		t.Fatalf("OnLLMEnd failed: %v", err)
	}

	enrichment, _ := store.Get("run-2")
	history := enrichment.GetOperationHistory()

	if len(history) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(history))
	}

	op := history[0]
	if op.LLM.ResponseID != "resp-123" {
		t.Errorf("ResponseID = %q, want %q", op.LLM.ResponseID, "resp-123")
	}
	if op.LLM.FinishReason != "stop" {
		t.Errorf("FinishReason = %q, want %q", op.LLM.FinishReason, "stop")
	}
	if op.LLM.PromptTokens != 50 {
		t.Errorf("PromptTokens = %d, want 50", op.LLM.PromptTokens)
	}
	if op.LLM.CompletionTokens != 25 {
		t.Errorf("CompletionTokens = %d, want 25", op.LLM.CompletionTokens)
	}
	if op.LLM.TotalTokens != 75 {
		t.Errorf("TotalTokens = %d, want 75", op.LLM.TotalTokens)
	}
	// Duration should be set (>= 0, can be 0 for very fast operations)
	if op.Duration < 0 {
		t.Errorf("Duration should be >= 0, got %d", op.Duration)
	}
}

// TestHookAdapterHandlesMissingLLMEnd verifies graceful handling when OnLLMEnd doesn't fire.
func TestHookAdapterHandlesMissingLLMEnd(t *testing.T) {
	store := NewEnrichmentStore()
	adapter := NewHookAdapter(store, nil, nil)

	ctx := aisen.WithRunID(context.Background(), "run-3")
	agent := agents.NewAgent(agents.AgentConfig{Name: "agent"})

	req := llmsdk.Request{
		Model:    "gpt-4",
		Messages: []llmsdk.Message{{Role: "user", Parts: []llmsdk.ContentPart{{Text: "Test"}}}},
	}

	// Start LLM call but never call OnLLMEnd
	adapter.OnLLMStart(ctx, nil, agent, req)

	enrichment, _ := store.Get("run-3")
	history := enrichment.GetOperationHistory()

	// Should still have the partial record
	if len(history) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(history))
	}

	op := history[0]
	if op.Kind != "llm" {
		t.Errorf("Kind = %q, want %q", op.Kind, "llm")
	}
	// Response fields should be empty/zero
	if op.LLM.ResponseID != "" {
		t.Errorf("ResponseID should be empty, got %q", op.LLM.ResponseID)
	}
	if op.Duration != 0 {
		t.Error("Duration should be 0 for incomplete operation")
	}
}

// TestHookAdapterCapturesToolCalls verifies tool metadata is captured in operation history.
func TestHookAdapterCapturesToolCalls(t *testing.T) {
	store := NewEnrichmentStore()
	adapter := NewHookAdapter(store, nil, nil)

	ctx := aisen.WithRunID(context.Background(), "run-tool-1")
	agent := agents.NewAgent(agents.AgentConfig{Name: "test-agent"})

	tool := agents.Tool{Name: "calculator"}
	call := llmsdk.ToolCall{
		ID:        "call-123",
		Name:      "calculator",
		Arguments: []byte(`{"op":"add","a":1,"b":2}`),
	}

	err := adapter.OnToolStart(ctx, nil, agent, tool, call)
	if err != nil {
		t.Fatalf("OnToolStart failed: %v", err)
	}

	enrichment, _ := store.Get("run-tool-1")
	history := enrichment.GetOperationHistory()

	if len(history) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(history))
	}

	op := history[0]
	if op.Kind != "tool" {
		t.Errorf("Kind = %q, want %q", op.Kind, "tool")
	}
	if op.AgentName != "test-agent" {
		t.Errorf("AgentName = %q, want %q", op.AgentName, "test-agent")
	}
	if op.Tool == nil {
		t.Fatal("Tool field is nil")
	}
	if op.Tool.Name != "calculator" {
		t.Errorf("Tool.Name = %q, want %q", op.Tool.Name, "calculator")
	}
	if op.Tool.CallID != "call-123" {
		t.Errorf("Tool.CallID = %q, want %q", op.Tool.CallID, "call-123")
	}
	if op.Tool.Input != `{"op":"add","a":1,"b":2}` {
		t.Errorf("Tool.Input = %q, want %q", op.Tool.Input, `{"op":"add","a":1,"b":2}`)
	}
	if op.Tool.InputSize != len(`{"op":"add","a":1,"b":2}`) {
		t.Errorf("Tool.InputSize = %d, want %d", op.Tool.InputSize, len(`{"op":"add","a":1,"b":2}`))
	}
}

// TestHookAdapterUpdatesToolOutput verifies output updates existing record.
func TestHookAdapterUpdatesToolOutput(t *testing.T) {
	store := NewEnrichmentStore()
	adapter := NewHookAdapter(store, nil, nil)

	ctx := aisen.WithRunID(context.Background(), "run-tool-2")
	agent := agents.NewAgent(agents.AgentConfig{Name: "agent"})

	tool := agents.Tool{Name: "search"}
	call := llmsdk.ToolCall{
		ID:        "call-456",
		Name:      "search",
		Arguments: []byte(`{"query":"test"}`),
	}

	// Start tool call
	adapter.OnToolStart(ctx, nil, agent, tool, call)

	// End tool call with output
	output := "search results here"
	err := adapter.OnToolEnd(ctx, nil, agent, tool, output)
	if err != nil {
		t.Fatalf("OnToolEnd failed: %v", err)
	}

	enrichment, _ := store.Get("run-tool-2")
	history := enrichment.GetOperationHistory()

	if len(history) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(history))
	}

	op := history[0]
	if op.Tool.Output != output {
		t.Errorf("Tool.Output = %q, want %q", op.Tool.Output, output)
	}
	if op.Tool.OutputSize != len(output) {
		t.Errorf("Tool.OutputSize = %d, want %d", op.Tool.OutputSize, len(output))
	}
	if op.Duration < 0 {
		t.Errorf("Duration should be >= 0, got %d", op.Duration)
	}
}

// TestHookAdapterHandlesMissingToolEnd verifies graceful handling when OnToolEnd doesn't fire.
func TestHookAdapterHandlesMissingToolEnd(t *testing.T) {
	store := NewEnrichmentStore()
	adapter := NewHookAdapter(store, nil, nil)

	ctx := aisen.WithRunID(context.Background(), "run-tool-3")
	agent := agents.NewAgent(agents.AgentConfig{Name: "agent"})

	tool := agents.Tool{Name: "search"}
	call := llmsdk.ToolCall{
		ID:        "call-789",
		Name:      "search",
		Arguments: []byte(`{"query":"test"}`),
	}

	// Start tool call but never call OnToolEnd
	adapter.OnToolStart(ctx, nil, agent, tool, call)

	enrichment, _ := store.Get("run-tool-3")
	history := enrichment.GetOperationHistory()

	// Should still have the partial record
	if len(history) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(history))
	}

	op := history[0]
	if op.Kind != "tool" {
		t.Errorf("Kind = %q, want %q", op.Kind, "tool")
	}
	// Output fields should be empty/zero
	if op.Tool.Output != "" {
		t.Errorf("Tool.Output should be empty, got %q", op.Tool.Output)
	}
	if op.Duration != 0 {
		t.Error("Duration should be 0 for incomplete operation")
	}
}
