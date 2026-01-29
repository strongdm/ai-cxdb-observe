// Tests for operation history tracking (ring buffer and operation records).
package agentssdk

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOperationHistoryBufferAdd verifies FIFO behavior when buffer is full.
func TestOperationHistoryBufferAdd(t *testing.T) {
	buffer := &operationHistoryBuffer{
		maxSize: 3,
	}

	// Add 3 records
	rec1 := OperationRecord{Kind: "llm", Timestamp: time.Now(), AgentName: "agent1"}
	rec2 := OperationRecord{Kind: "tool", Timestamp: time.Now(), AgentName: "agent2"}
	rec3 := OperationRecord{Kind: "llm", Timestamp: time.Now(), AgentName: "agent3"}

	buffer.Add(rec1)
	buffer.Add(rec2)
	buffer.Add(rec3)

	assert.Equal(t, 3, len(buffer.records), "buffer should have 3 records")

	// Add 4th record - should evict oldest (rec1)
	rec4 := OperationRecord{Kind: "tool", Timestamp: time.Now(), AgentName: "agent4"}
	buffer.Add(rec4)

	assert.Equal(t, 3, len(buffer.records), "buffer should still have 3 records")

	// Verify oldest was evicted
	all := buffer.GetAll()
	assert.Equal(t, 3, len(all), "should return 3 records")
	assert.Equal(t, "agent2", all[0].AgentName, "first record should be agent2 (oldest)")
	assert.Equal(t, "agent3", all[1].AgentName, "second record should be agent3")
	assert.Equal(t, "agent4", all[2].AgentName, "third record should be agent4 (newest)")
}

// TestOperationHistoryBufferGetAll verifies chronological ordering.
func TestOperationHistoryBufferGetAll(t *testing.T) {
	buffer := &operationHistoryBuffer{
		maxSize: 3,
	}

	// Empty buffer
	all := buffer.GetAll()
	assert.Empty(t, all, "empty buffer should return empty slice")

	// Add records
	rec1 := OperationRecord{Kind: "llm", AgentName: "first"}
	rec2 := OperationRecord{Kind: "tool", AgentName: "second"}
	rec3 := OperationRecord{Kind: "llm", AgentName: "third"}

	buffer.Add(rec1)
	buffer.Add(rec2)
	buffer.Add(rec3)

	all = buffer.GetAll()
	require.Equal(t, 3, len(all))
	assert.Equal(t, "first", all[0].AgentName)
	assert.Equal(t, "second", all[1].AgentName)
	assert.Equal(t, "third", all[2].AgentName)

	// Add more to trigger wrap-around
	rec4 := OperationRecord{Kind: "tool", AgentName: "fourth"}
	rec5 := OperationRecord{Kind: "llm", AgentName: "fifth"}
	buffer.Add(rec4)
	buffer.Add(rec5)

	all = buffer.GetAll()
	require.Equal(t, 3, len(all))
	// Should have: third, fourth, fifth (oldest to newest)
	assert.Equal(t, "third", all[0].AgentName)
	assert.Equal(t, "fourth", all[1].AgentName)
	assert.Equal(t, "fifth", all[2].AgentName)
}

// TestOperationHistoryBufferBounds verifies maxSize enforcement.
func TestOperationHistoryBufferBounds(t *testing.T) {
	buffer := &operationHistoryBuffer{
		maxSize: 2,
	}

	// Add more than maxSize
	for i := 0; i < 10; i++ {
		rec := OperationRecord{Kind: "llm", AgentName: "agent"}
		buffer.Add(rec)
	}

	assert.Equal(t, 2, len(buffer.records), "buffer should never exceed maxSize")
	all := buffer.GetAll()
	assert.Equal(t, 2, len(all), "GetAll should return maxSize records")
}

// TestOperationRecordJSONSerialization verifies JSON round-trip.
func TestOperationRecordJSONSerialization(t *testing.T) {
	now := time.Now().Round(time.Millisecond) // Round to avoid precision issues

	rec := OperationRecord{
		Kind:      "llm",
		Timestamp: now,
		Duration:  1234,
		AgentName: "test-agent",
		LLM: &LLMOperation{
			Model:            "claude-3",
			Provider:         "anthropic",
			MessageCount:     2,
			PromptTokens:     100,
			CompletionTokens: 50,
			TotalTokens:      150,
		},
	}

	// Marshal to JSON
	data, err := json.Marshal(rec)
	require.NoError(t, err, "marshaling should succeed")

	// Unmarshal back
	var decoded OperationRecord
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err, "unmarshaling should succeed")

	// Verify fields
	assert.Equal(t, "llm", decoded.Kind)
	assert.Equal(t, "test-agent", decoded.AgentName)
	assert.Equal(t, int64(1234), decoded.Duration)
	assert.Equal(t, now.Unix(), decoded.Timestamp.Unix()) // Compare Unix timestamps
	require.NotNil(t, decoded.LLM)
	assert.Equal(t, "claude-3", decoded.LLM.Model)
	assert.Equal(t, 100, decoded.LLM.PromptTokens)
	assert.Equal(t, 50, decoded.LLM.CompletionTokens)
	assert.Equal(t, 150, decoded.LLM.TotalTokens)
}

// TestOperationRecordToolJSONSerialization verifies tool operation JSON.
func TestOperationRecordToolJSONSerialization(t *testing.T) {
	rec := OperationRecord{
		Kind:      "tool",
		Timestamp: time.Now(),
		Duration:  500,
		Tool: &ToolOperation{
			Name:       "calculator",
			CallID:     "call_123",
			InputSize:  42,
			OutputSize: 10,
			Input:      `{"op":"add","a":1,"b":2}`,
			Output:     "3",
		},
	}

	data, err := json.Marshal(rec)
	require.NoError(t, err)

	var decoded OperationRecord
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, "tool", decoded.Kind)
	require.NotNil(t, decoded.Tool)
	assert.Equal(t, "calculator", decoded.Tool.Name)
	assert.Equal(t, "call_123", decoded.Tool.CallID)
	assert.Equal(t, 42, decoded.Tool.InputSize)
	assert.Equal(t, 10, decoded.Tool.OutputSize)
	assert.Equal(t, `{"op":"add","a":1,"b":2}`, decoded.Tool.Input)
	assert.Equal(t, "3", decoded.Tool.Output)
}

// TestMessageMetadataJSON verifies message metadata serialization.
func TestMessageMetadataJSON(t *testing.T) {
	msg := MessageMetadata{
		Role:          "user",
		ContentLength: 1024,
		PartsCount:    2,
		HasImage:      true,
		HasToolCall:   false,
		HasToolResult: false,
	}

	data, err := json.Marshal(msg)
	require.NoError(t, err)

	var decoded MessageMetadata
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, "user", decoded.Role)
	assert.Equal(t, 1024, decoded.ContentLength)
	assert.Equal(t, 2, decoded.PartsCount)
	assert.True(t, decoded.HasImage)
	assert.False(t, decoded.HasToolCall)
	assert.False(t, decoded.HasToolResult)
}

// TestOperationRecordWithError verifies error field serialization.
func TestOperationRecordWithError(t *testing.T) {
	rec := OperationRecord{
		Kind:      "llm",
		Timestamp: time.Now(),
		Error:     "context_length_exceeded",
	}

	data, err := json.Marshal(rec)
	require.NoError(t, err)

	var decoded OperationRecord
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, "context_length_exceeded", decoded.Error)
}
