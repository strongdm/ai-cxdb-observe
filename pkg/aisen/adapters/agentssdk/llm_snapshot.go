// Helper functions to build LLM operation snapshots without storing full message text.
package agentssdk

import (
	llmsdk "github.com/strongdm/ai-llm-sdk/pkg/llm"
)

// buildLLMOperation extracts metadata from an LLM request.
// SECURITY: Does NOT store full message text, only metadata.
func buildLLMOperation(req llmsdk.Request) *LLMOperation {
	op := &LLMOperation{
		Model:        req.Model,
		Provider:     string(req.Provider),
		MessageCount: len(req.Messages),
		Temperature:  req.Temperature,
		TopP:         req.TopP,
		MaxTokens:    req.MaxTokens,
		ToolCount:    len(req.Tools),
	}

	// Extract tool names only (not schemas)
	if len(req.Tools) > 0 {
		op.ToolNames = make([]string, len(req.Tools))
		for i, tool := range req.Tools {
			op.ToolNames[i] = tool.Name
		}
	}

	// Build message metadata (last 10 messages)
	messageCount := len(req.Messages)
	startIdx := 0
	if messageCount > 10 {
		startIdx = messageCount - 10
	}

	op.Messages = make([]MessageMetadata, 0, messageCount-startIdx)
	for i := startIdx; i < messageCount; i++ {
		op.Messages = append(op.Messages, buildMessageMetadata(req.Messages[i]))
	}

	return op
}

// buildMessageMetadata extracts metadata from a message without storing content.
// SECURITY: Full text is NOT stored to avoid leaking prompts/secrets.
func buildMessageMetadata(msg llmsdk.Message) MessageMetadata {
	metadata := MessageMetadata{
		Role:       string(msg.Role),
		PartsCount: len(msg.Parts),
	}

	// Calculate content length and detect special content types
	contentLength := 0
	for _, part := range msg.Parts {
		contentLength += len(part.Text)
		if part.ImageData != nil {
			metadata.HasImage = true
		}
		if part.ToolCall != nil {
			metadata.HasToolCall = true
		}
		if part.ToolResult != nil {
			metadata.HasToolResult = true
		}
	}
	metadata.ContentLength = contentLength

	return metadata
}

// updateLLMOperationWithResponse updates an LLMOperation with response metadata.
func updateLLMOperationWithResponse(op *LLMOperation, resp llmsdk.Response) {
	if op == nil {
		return
	}

	op.ResponseID = resp.ID
	op.FinishReason = string(resp.FinishReason)
	op.PromptTokens = resp.Usage.PromptTokens
	op.CompletionTokens = resp.Usage.CompletionTokens
	op.TotalTokens = resp.Usage.TotalTokens

	// Extract tool call names from response
	if len(resp.ToolCalls) > 0 {
		op.ToolCallCount = len(resp.ToolCalls)
		op.ToolCallNames = make([]string, len(resp.ToolCalls))
		for i, tc := range resp.ToolCalls {
			op.ToolCallNames[i] = tc.Name
		}
	}
}
