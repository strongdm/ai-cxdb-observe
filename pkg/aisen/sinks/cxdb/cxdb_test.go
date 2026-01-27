package cxdb

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/strongdm/ai-cxdb-observe/pkg/aisen"
	cxdbclient "github.com/strongdm/ai-cxdb/clients/go"
	cxdtypes "github.com/strongdm/ai-cxdb/clients/go/types"
)

// mockCXDBClient is a test double for the cxdb client.
type mockCXDBClient struct {
	mu             sync.Mutex
	createContexts []uint64 // baseTurnIDs passed to CreateContext
	appendRequests []*cxdbclient.AppendRequest
	nextContextID  uint64
	createErr      error
	appendErr      error
}

func (m *mockCXDBClient) CreateContext(ctx context.Context, baseTurnID uint64) (*cxdbclient.ContextHead, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.createErr != nil {
		return nil, m.createErr
	}
	m.createContexts = append(m.createContexts, baseTurnID)
	m.nextContextID++
	return &cxdbclient.ContextHead{
		ContextID:  m.nextContextID,
		HeadTurnID: 0,
		HeadDepth:  0,
	}, nil
}

func (m *mockCXDBClient) AppendTurn(ctx context.Context, req *cxdbclient.AppendRequest) (*cxdbclient.AppendResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.appendErr != nil {
		return nil, m.appendErr
	}
	m.appendRequests = append(m.appendRequests, req)
	return &cxdbclient.AppendResult{
		ContextID: req.ContextID,
		TurnID:    1,
		Depth:     1,
	}, nil
}

func (m *mockCXDBClient) getAppendRequests() []*cxdbclient.AppendRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*cxdbclient.AppendRequest, len(m.appendRequests))
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

func decodeConversationItem(t *testing.T, payload []byte) cxdtypes.ConversationItem {
	t.Helper()
	var item cxdtypes.ConversationItem
	if err := cxdbclient.DecodeMsgpackInto(payload, &item); err != nil {
		t.Fatalf("DecodeMsgpackInto failed: %v", err)
	}
	return item
}

func decodeDetailsJSON(t *testing.T, content string) map[string]any {
	t.Helper()
	var details map[string]any
	if err := json.Unmarshal([]byte(content), &details); err != nil {
		t.Fatalf("details JSON unmarshal failed: %v", err)
	}
	return details
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
	if req.TypeID != cxdtypes.TypeIDConversationItem {
		t.Errorf("TypeID = %q, want %q", req.TypeID, cxdtypes.TypeIDConversationItem)
	}
	if req.TypeVersion != cxdtypes.TypeVersionConversationItem {
		t.Errorf("TypeVersion = %d, want %d", req.TypeVersion, cxdtypes.TypeVersionConversationItem)
	}
	if req.IdempotencyKey != "evt-123" {
		t.Errorf("IdempotencyKey = %q, want %q", req.IdempotencyKey, "evt-123")
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

	appendReqs := client.getAppendRequests()
	if len(appendReqs) != 1 {
		t.Fatalf("Expected 1 append request, got %d", len(appendReqs))
	}

	item := decodeConversationItem(t, appendReqs[0].Payload)
	if item.ContextMetadata == nil {
		t.Fatalf("ContextMetadata should be set for orphan contexts")
	}
	if item.ContextMetadata.ClientTag == "" {
		t.Errorf("ClientTag should be set for orphan contexts")
	}
	if len(item.ContextMetadata.Labels) == 0 {
		t.Errorf("Labels should be set for orphan contexts")
	}
}

func TestCXDBSink_Write_PayloadFormat_CanonicalTypes(t *testing.T) {
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
	item := decodeConversationItem(t, req.Payload)

	if item.ItemType != cxdtypes.ItemTypeSystem {
		t.Errorf("ItemType = %q, want %q", item.ItemType, cxdtypes.ItemTypeSystem)
	}
	if item.Status != cxdtypes.ItemStatusComplete {
		t.Errorf("Status = %q, want %q", item.Status, cxdtypes.ItemStatusComplete)
	}
	if item.System == nil {
		t.Fatalf("System message should be present")
	}
	if item.System.Kind != cxdtypes.SystemKindError {
		t.Errorf("System.Kind = %q, want %q", item.System.Kind, cxdtypes.SystemKindError)
	}

	details := decodeDetailsJSON(t, item.System.Content)
	if details["event_id"] != "evt-456" {
		t.Errorf("event_id = %v, want evt-456", details["event_id"])
	}
	if details["fingerprint"] != "fp123" {
		t.Errorf("fingerprint = %v, want fp123", details["fingerprint"])
	}
	if details["operation"] != "tool" {
		t.Errorf("operation = %v, want tool", details["operation"])
	}
	if details["agent_name"] != "agent1" {
		t.Errorf("agent_name = %v, want agent1", details["agent_name"])
	}
	if details["tool_name"] != "WebSearch" {
		t.Errorf("tool_name = %v, want WebSearch", details["tool_name"])
	}

	// Non-orphan contexts should not include context metadata.
	if item.ContextMetadata != nil {
		t.Errorf("ContextMetadata should be nil for non-orphan contexts")
	}
}

func TestCXDBSink_WithOrphanLabels_AndClientTag(t *testing.T) {
	client := &mockCXDBClient{}
	sink := NewCXDBSink(
		client,
		WithOrphanLabels([]string{"error", "critical"}),
		WithClientTag("aisen-e2e"),
	)

	event := aisen.ErrorEvent{
		EventID:   "evt-789",
		Timestamp: time.Now(),
		Severity:  aisen.SeverityError,
		ErrorType: "test",
		Message:   "boom",
		// No ContextID - will create orphan
	}

	err := sink.Write(context.Background(), event)
	if err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	appendReqs := client.getAppendRequests()
	if len(appendReqs) != 1 {
		t.Fatalf("Expected 1 append request, got %d", len(appendReqs))
	}

	item := decodeConversationItem(t, appendReqs[0].Payload)
	if item.ContextMetadata == nil {
		t.Fatalf("ContextMetadata should be set for orphan contexts")
	}
	if item.ContextMetadata.ClientTag != "aisen-e2e" {
		t.Errorf("ClientTag = %q, want %q", item.ContextMetadata.ClientTag, "aisen-e2e")
	}
	if len(item.ContextMetadata.Labels) != 2 || item.ContextMetadata.Labels[1] != "critical" {
		t.Errorf("Labels = %v, want %v", item.ContextMetadata.Labels, []string{"error", "critical"})
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
