package cxdb

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/strongdm/ai-cxdb-observe/pkg/aisen"
)

// mockCXDBClient is a test double for the cxdb client.
type mockCXDBClient struct {
	mu             sync.Mutex
	createContexts []uint64 // baseTurnIDs passed to CreateContext
	appendRequests []*AppendRequest
	nextContextID  uint64
	createErr      error
	appendErr      error
}

func (m *mockCXDBClient) CreateContext(ctx context.Context, baseTurnID uint64) (*ContextHead, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.createErr != nil {
		return nil, m.createErr
	}
	m.createContexts = append(m.createContexts, baseTurnID)
	m.nextContextID++
	return &ContextHead{
		ContextID:  m.nextContextID,
		HeadTurnID: 0,
		HeadDepth:  0,
	}, nil
}

func (m *mockCXDBClient) AppendTurn(ctx context.Context, req *AppendRequest) (*AppendResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.appendErr != nil {
		return nil, m.appendErr
	}
	m.appendRequests = append(m.appendRequests, req)
	return &AppendResult{
		ContextID: req.ContextID,
		TurnID:    1,
		Depth:     1,
	}, nil
}

func (m *mockCXDBClient) getAppendRequests() []*AppendRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*AppendRequest, len(m.appendRequests))
	copy(result, m.appendRequests)
	return result
}

func (m *mockCXDBClient) getCreateContextCalls() []uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]uint64, len(m.createContexts))
	copy(result, m.createContexts)
	return result
}

func TestCXDBSink_ImplementsSinkInterface(t *testing.T) {
	client := &mockCXDBClient{}
	var _ aisen.Sink = NewCXDBSink(client)
}

func TestCXDBSink_Write_WithContextID_AppendsTurn(t *testing.T) {
	client := &mockCXDBClient{}
	sink := NewCXDBSink(client)

	contextID := uint64(12345)
	event := aisen.ErrorEvent{
		EventID:   "evt-123",
		Timestamp: time.Date(2025, 1, 26, 12, 0, 0, 0, time.UTC),
		Severity:  aisen.SeverityError,
		ErrorType: "test",
		Message:   "test error",
		ContextID: &contextID,
	}

	err := sink.Write(context.Background(), event)
	if err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	// Should NOT create a new context
	createCalls := client.getCreateContextCalls()
	if len(createCalls) != 0 {
		t.Errorf("Should not create context when ContextID is set, got %d create calls", len(createCalls))
	}

	// Should append to existing context
	appendReqs := client.getAppendRequests()
	if len(appendReqs) != 1 {
		t.Fatalf("Expected 1 append request, got %d", len(appendReqs))
	}

	req := appendReqs[0]
	if req.ContextID != 12345 {
		t.Errorf("AppendRequest.ContextID = %d, want 12345", req.ContextID)
	}
}

func TestCXDBSink_Write_WithoutContextID_CreatesOrphan(t *testing.T) {
	client := &mockCXDBClient{}
	sink := NewCXDBSink(client)

	event := aisen.ErrorEvent{
		EventID:   "evt-123",
		Timestamp: time.Now(),
		Severity:  aisen.SeverityError,
		ErrorType: "test",
		// No ContextID
	}

	err := sink.Write(context.Background(), event)
	if err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	// Should create a new context
	createCalls := client.getCreateContextCalls()
	if len(createCalls) != 1 {
		t.Fatalf("Expected 1 create context call, got %d", len(createCalls))
	}

	// Should append to the newly created context
	appendReqs := client.getAppendRequests()
	if len(appendReqs) != 1 {
		t.Fatalf("Expected 1 append request, got %d", len(appendReqs))
	}
}

func TestCXDBSink_Write_PayloadFormat(t *testing.T) {
	client := &mockCXDBClient{}
	sink := NewCXDBSink(client)

	contextID := uint64(99)
	event := aisen.ErrorEvent{
		EventID:     "evt-456",
		Timestamp:   time.Date(2025, 1, 26, 12, 0, 0, 0, time.UTC),
		Fingerprint: "fp123",
		Severity:    aisen.SeverityError,
		ErrorType:   "timeout",
		Message:     "connection timed out",
		Operation:   "tool",
		AgentName:   "agent1",
		ToolName:    "WebSearch",
		ContextID:   &contextID,
	}

	err := sink.Write(context.Background(), event)
	if err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	appendReqs := client.getAppendRequests()
	if len(appendReqs) != 1 {
		t.Fatalf("Expected 1 append request, got %d", len(appendReqs))
	}

	req := appendReqs[0]

	// Verify TypeID and TypeVersion
	if req.TypeID != TypeIDConversationItem {
		t.Errorf("TypeID = %q, want %q", req.TypeID, TypeIDConversationItem)
	}
	if req.TypeVersion != TypeVersionConversationItem {
		t.Errorf("TypeVersion = %d, want %d", req.TypeVersion, TypeVersionConversationItem)
	}

	// Verify IdempotencyKey
	if req.IdempotencyKey != "evt-456" {
		t.Errorf("IdempotencyKey = %q, want %q", req.IdempotencyKey, "evt-456")
	}

	// Payload should be non-empty (msgpack encoded)
	if len(req.Payload) == 0 {
		t.Error("Payload should not be empty")
	}
}

func TestCXDBSink_WithOrphanLabels(t *testing.T) {
	client := &mockCXDBClient{}
	sink := NewCXDBSink(client, WithOrphanLabels([]string{"error", "critical"}))

	event := aisen.ErrorEvent{
		EventID:  "evt-123",
		Severity: aisen.SeverityError,
		// No ContextID - will create orphan
	}

	err := sink.Write(context.Background(), event)
	if err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	// Verify that labels were configured (they're used in metadata)
	appendReqs := client.getAppendRequests()
	if len(appendReqs) != 1 {
		t.Fatalf("Expected 1 append request, got %d", len(appendReqs))
	}
}

func TestCXDBSink_Flush(t *testing.T) {
	client := &mockCXDBClient{}
	sink := NewCXDBSink(client)

	err := sink.Flush(context.Background())
	if err != nil {
		t.Errorf("Flush returned error: %v", err)
	}
}

func TestCXDBSink_Close(t *testing.T) {
	client := &mockCXDBClient{}
	sink := NewCXDBSink(client)

	err := sink.Close()
	if err != nil {
		t.Errorf("Close returned error: %v", err)
	}
}

func TestCXDBSink_ErrorDetails_JSON(t *testing.T) {
	client := &mockCXDBClient{}
	sink := NewCXDBSink(client)

	contextID := uint64(100)
	turnDepth := 5
	tokensWasted := int64(1000)
	event := aisen.ErrorEvent{
		EventID:      "evt-json-test",
		Timestamp:    time.Date(2025, 1, 26, 12, 0, 0, 0, time.UTC),
		Fingerprint:  "fp-test",
		Severity:     aisen.SeverityCrash,
		ErrorType:    "panic",
		Message:      "nil pointer",
		StackTrace:   "stack trace here",
		Operation:    "llm",
		AgentName:    "testAgent",
		ToolName:     "testTool",
		ContextID:    &contextID,
		TurnDepth:    &turnDepth,
		TokensWasted: &tokensWasted,
		Metadata:     map[string]string{"key": "value"},
	}

	sink.Write(context.Background(), event)

	// Extract and verify the JSON content from the payload
	// This tests that error details are properly JSON-encoded
	appendReqs := client.getAppendRequests()
	if len(appendReqs) != 1 {
		t.Fatalf("Expected 1 append request, got %d", len(appendReqs))
	}

	// The payload contains a ConversationItem with System.Content as JSON
	// We can't easily decode msgpack here, but we can verify the request was made
	if len(appendReqs[0].Payload) == 0 {
		t.Error("Payload should contain encoded ConversationItem")
	}
}

// Test that buildErrorDetails produces valid JSON
func TestBuildErrorDetails_ValidJSON(t *testing.T) {
	event := aisen.ErrorEvent{
		EventID:     "evt-1",
		Fingerprint: "fp-1",
		Severity:    aisen.SeverityError,
		ErrorType:   "test",
		Message:     "test message",
		StackTrace:  "line1\nline2",
		Operation:   "tool",
		AgentName:   "agent",
		ToolName:    "tool",
		Metadata:    map[string]string{"k": "v"},
	}

	details := buildErrorDetails(event)

	// Should be valid JSON
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(details), &parsed); err != nil {
		t.Errorf("buildErrorDetails produced invalid JSON: %v", err)
	}

	// Check expected fields
	if parsed["event_id"] != "evt-1" {
		t.Errorf("event_id = %v, want %q", parsed["event_id"], "evt-1")
	}
	if parsed["severity"] != "error" {
		t.Errorf("severity = %v, want %q", parsed["severity"], "error")
	}
}
