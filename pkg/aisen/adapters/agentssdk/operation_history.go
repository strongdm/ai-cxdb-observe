// Operation history tracking for aisen - captures LLM and tool operations in a ring buffer.
package agentssdk

import (
	"time"
)

// OperationRecord captures a single operation (LLM call or tool call).
// Stored as JSON in ErrorEvent.Metadata["aisen.operation_history_json"].
type OperationRecord struct {
	Kind      string    `json:"kind"`                   // "llm" or "tool"
	Timestamp time.Time `json:"timestamp"`              // When operation started
	Duration  int64     `json:"duration_ms,omitempty"`  // Duration in milliseconds
	AgentName string    `json:"agent_name,omitempty"`

	// LLM-specific fields (populated when Kind == "llm")
	LLM *LLMOperation `json:"llm,omitempty"`

	// Tool-specific fields (populated when Kind == "tool")
	Tool *ToolOperation `json:"tool,omitempty"`

	// Error field (if operation failed)
	Error string `json:"error,omitempty"`
}

// LLMOperation captures metadata from an LLM call.
// SECURITY: Does NOT store full message text, only metadata.
type LLMOperation struct {
	// Request metadata
	Model        string            `json:"model"`
	Provider     string            `json:"provider"`
	MessageCount int               `json:"message_count"`
	Messages     []MessageMetadata `json:"messages"`               // Metadata only
	Temperature  *float32          `json:"temperature,omitempty"`
	TopP         *float32          `json:"top_p,omitempty"`
	MaxTokens    *int              `json:"max_tokens,omitempty"`
	ToolCount    int               `json:"tool_count"`
	ToolNames    []string          `json:"tool_names,omitempty"`   // Names only

	// Response metadata (populated by OnLLMEnd)
	ResponseID       string   `json:"response_id,omitempty"`
	FinishReason     string   `json:"finish_reason,omitempty"`
	ToolCallCount    int      `json:"tool_call_count,omitempty"`
	ToolCallNames    []string `json:"tool_call_names,omitempty"`
	PromptTokens     int      `json:"prompt_tokens,omitempty"`
	CompletionTokens int      `json:"completion_tokens,omitempty"`
	TotalTokens      int      `json:"total_tokens,omitempty"`
}

// MessageMetadata captures message structure without content.
// SECURITY: Full text is NOT stored to avoid leaking prompts/secrets.
type MessageMetadata struct {
	Role          string `json:"role"`                      // "system", "user", "assistant", "tool"
	ContentLength int    `json:"content_length"`            // Character count
	PartsCount    int    `json:"parts_count"`               // Number of content parts
	HasImage      bool   `json:"has_image,omitempty"`
	HasToolCall   bool   `json:"has_tool_call,omitempty"`
	HasToolResult bool   `json:"has_tool_result,omitempty"`
}

// ToolOperation captures metadata from a tool call.
type ToolOperation struct {
	Name       string `json:"name"`
	CallID     string `json:"call_id"`
	InputSize  int    `json:"input_size"`             // Byte size of arguments
	OutputSize int    `json:"output_size,omitempty"`  // Byte size of output
	// Note: Actual input/output JSON stored in separate scrubbed fields
	Input  string `json:"input,omitempty"`            // Scrubbed JSON string
	Output string `json:"output,omitempty"`           // Scrubbed output string
}

// operationHistoryBuffer is a bounded ring buffer (internal implementation).
type operationHistoryBuffer struct {
	records  []OperationRecord
	maxSize  int
	writeIdx int
}

// Add appends a record, evicting oldest if buffer is full.
func (b *operationHistoryBuffer) Add(record OperationRecord) {
	if len(b.records) < b.maxSize {
		// Buffer not full yet, just append
		b.records = append(b.records, record)
	} else {
		// Buffer full, overwrite at writeIdx
		b.records[b.writeIdx] = record
		b.writeIdx = (b.writeIdx + 1) % b.maxSize
	}
}

// GetAll returns records in chronological order (oldest first).
func (b *operationHistoryBuffer) GetAll() []OperationRecord {
	if len(b.records) == 0 {
		return []OperationRecord{}
	}

	if len(b.records) < b.maxSize {
		// Buffer not full yet, return as-is (already chronological)
		return b.records
	}

	// Buffer is full, need to reorder
	// writeIdx points to oldest record
	result := make([]OperationRecord, len(b.records))
	copy(result, b.records[b.writeIdx:])
	copy(result[len(b.records)-b.writeIdx:], b.records[:b.writeIdx])
	return result
}

// UpdateLast updates the last (most recently added) record.
// Returns true if update succeeded, false if buffer is empty.
func (b *operationHistoryBuffer) UpdateLast(fn func(*OperationRecord)) bool {
	if len(b.records) == 0 {
		return false
	}

	// Find index of last record
	var lastIdx int
	if len(b.records) < b.maxSize {
		// Buffer not full yet, last is at end
		lastIdx = len(b.records) - 1
	} else {
		// Buffer is full, last is just before writeIdx
		lastIdx = (b.writeIdx - 1 + b.maxSize) % b.maxSize
	}

	fn(&b.records[lastIdx])
	return true
}
