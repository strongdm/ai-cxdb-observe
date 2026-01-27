// Package cxdb provides a sink that persists errors to cxdb as SystemMessage items.
package cxdb

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/strongdm/ai-cxdb-observe/pkg/aisen"
	"github.com/vmihailenco/msgpack/v5"
)

// Type constants matching ai-cxdb/clients/go/types/conversation.go
const (
	// TypeIDConversationItem is the type ID for ConversationItem.
	TypeIDConversationItem = "cxdb.ConversationItem"

	// TypeVersionConversationItem is the current schema version.
	TypeVersionConversationItem uint32 = 3

	// ItemType constants
	itemTypeSystem = "system"

	// ItemStatus constants
	itemStatusComplete = "complete"

	// SystemKind constants
	systemKindError = "error"
)

// ContextHead represents a context's current state.
type ContextHead struct {
	ContextID  uint64
	HeadTurnID uint64
	HeadDepth  uint32
}

// AppendRequest contains parameters for appending a turn.
type AppendRequest struct {
	ContextID      uint64
	ParentTurnID   uint64
	TypeID         string
	TypeVersion    uint32
	Payload        []byte
	IdempotencyKey string
}

// AppendResult contains the result of an append operation.
type AppendResult struct {
	ContextID   uint64
	TurnID      uint64
	Depth       uint32
	PayloadHash [32]byte
}

// CXDBClient is the minimal interface for cxdb client operations.
// The real *cxdb.Client satisfies this interface.
type CXDBClient interface {
	CreateContext(ctx context.Context, baseTurnID uint64) (*ContextHead, error)
	AppendTurn(ctx context.Context, req *AppendRequest) (*AppendResult, error)
}

// CXDBSinkOption configures the CXDB sink.
type CXDBSinkOption func(*cxdbSinkConfig)

type cxdbSinkConfig struct {
	orphanLabels []string
	clientTag    string
}

// WithOrphanLabels sets labels for orphan error contexts.
func WithOrphanLabels(labels []string) CXDBSinkOption {
	return func(c *cxdbSinkConfig) {
		c.orphanLabels = labels
	}
}

// WithClientTag sets the client tag for orphan contexts.
func WithClientTag(tag string) CXDBSinkOption {
	return func(c *cxdbSinkConfig) {
		c.clientTag = tag
	}
}

// cxdbSink writes errors to cxdb as SystemMessage items.
type cxdbSink struct {
	client       CXDBClient
	orphanLabels []string
	clientTag    string
}

// NewCXDBSink creates a sink that writes to cxdb.
func NewCXDBSink(client CXDBClient, opts ...CXDBSinkOption) aisen.Sink {
	cfg := &cxdbSinkConfig{
		orphanLabels: []string{"error", "unlinked"},
		clientTag:    "aisen",
	}
	for _, opt := range opts {
		opt(cfg)
	}

	return &cxdbSink{
		client:       client,
		orphanLabels: cfg.orphanLabels,
		clientTag:    cfg.clientTag,
	}
}

// Write persists an error event to cxdb.
func (s *cxdbSink) Write(ctx context.Context, event aisen.ErrorEvent) error {
	var contextID uint64
	isOrphan := false

	if event.ContextID != nil {
		contextID = *event.ContextID
	} else {
		// Create orphan context
		head, err := s.client.CreateContext(ctx, 0)
		if err != nil {
			return fmt.Errorf("create orphan context: %w", err)
		}
		contextID = head.ContextID
		isOrphan = true
	}

	// Build the ConversationItem payload
	item := s.buildConversationItem(event, isOrphan)

	// Encode to msgpack
	payload, err := encodeMsgpack(item)
	if err != nil {
		return fmt.Errorf("encode payload: %w", err)
	}

	// Append to context
	req := &AppendRequest{
		ContextID:      contextID,
		TypeID:         TypeIDConversationItem,
		TypeVersion:    TypeVersionConversationItem,
		Payload:        payload,
		IdempotencyKey: event.EventID,
	}

	_, err = s.client.AppendTurn(ctx, req)
	if err != nil {
		return fmt.Errorf("append turn: %w", err)
	}

	return nil
}

// conversationItem mirrors types.ConversationItem structure for encoding.
// Uses msgpack tags matching the cxdb types package.
type conversationItem struct {
	ItemType        string            `msgpack:"1"`
	Status          string            `msgpack:"2"`
	Timestamp       int64             `msgpack:"3"`
	ID              string            `msgpack:"4"`
	System          *systemMessage    `msgpack:"9,omitempty"`
	ContextMetadata *contextMetadata  `msgpack:"15,omitempty"`
}

type systemMessage struct {
	Kind    string `msgpack:"1"`
	Title   string `msgpack:"2,omitempty"`
	Content string `msgpack:"3,omitempty"`
}

type contextMetadata struct {
	Labels    []string `msgpack:"1,omitempty"`
	ClientTag string   `msgpack:"2,omitempty"`
}

// buildConversationItem creates a ConversationItem from an ErrorEvent.
func (s *cxdbSink) buildConversationItem(event aisen.ErrorEvent, isOrphan bool) *conversationItem {
	// Build title: "error_type: truncated_message"
	title := event.ErrorType
	if event.Message != "" {
		maxMsgLen := 80
		msg := event.Message
		if len(msg) > maxMsgLen {
			msg = msg[:maxMsgLen] + "..."
		}
		title = event.ErrorType + ": " + msg
	}

	// Truncate title to 100 chars
	if len(title) > 100 {
		title = title[:97] + "..."
	}

	item := &conversationItem{
		ItemType:  itemTypeSystem,
		Status:    itemStatusComplete,
		Timestamp: event.Timestamp.UnixMilli(),
		ID:        event.EventID,
		System: &systemMessage{
			Kind:    systemKindError,
			Title:   title,
			Content: buildErrorDetails(event),
		},
	}

	// Add context metadata for orphan contexts
	if isOrphan {
		item.ContextMetadata = &contextMetadata{
			Labels:    s.orphanLabels,
			ClientTag: s.clientTag,
		}
	}

	return item
}

// buildErrorDetails encodes the full ErrorEvent as JSON for SystemMessage.Content.
func buildErrorDetails(event aisen.ErrorEvent) string {
	details := map[string]interface{}{
		"event_id":    event.EventID,
		"severity":    string(event.Severity),
		"error_type":  event.ErrorType,
		"message":     event.Message,
		"fingerprint": event.Fingerprint,
		"operation":   event.Operation,
	}

	if event.StackTrace != "" {
		details["stack_trace"] = event.StackTrace
	}
	if event.OperationID != "" {
		details["operation_id"] = event.OperationID
	}
	if event.AgentName != "" {
		details["agent_name"] = event.AgentName
	}
	if event.ToolName != "" {
		details["tool_name"] = event.ToolName
	}
	if event.ToolArgs != "" {
		details["tool_args"] = event.ToolArgs
	}
	if event.ContextID != nil {
		details["context_id"] = *event.ContextID
	}
	if event.TurnDepth != nil {
		details["turn_depth"] = *event.TurnDepth
	}
	if event.SystemState != nil {
		details["system_state"] = map[string]interface{}{
			"memory_bytes":    event.SystemState.MemoryBytes,
			"goroutine_count": event.SystemState.GoroutineCount,
			"uptime_ms":       event.SystemState.UptimeMs,
			"host_name":       event.SystemState.HostName,
		}
	}
	if event.TokensWasted != nil {
		details["tokens_wasted"] = *event.TokensWasted
	}
	if len(event.Metadata) > 0 {
		details["metadata"] = event.Metadata
	}

	jsonBytes, err := json.Marshal(details)
	if err != nil {
		// Fallback to simple error message
		return fmt.Sprintf(`{"error":"failed to encode details: %s"}`, err)
	}
	return string(jsonBytes)
}

// encodeMsgpack encodes a value to msgpack bytes.
func encodeMsgpack(v interface{}) ([]byte, error) {
	return msgpack.Marshal(v)
}

// Flush is a no-op for the cxdb sink (writes are synchronous).
func (s *cxdbSink) Flush(ctx context.Context) error {
	return nil
}

// Close is a no-op for the cxdb sink.
func (s *cxdbSink) Close() error {
	return nil
}
