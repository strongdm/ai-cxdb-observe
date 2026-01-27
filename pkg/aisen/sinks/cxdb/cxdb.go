// Package cxdb provides a sink that persists errors to cxdb as SystemMessage items.
package cxdb

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/strongdm/ai-cxdb-observe/pkg/aisen"
	cxdbclient "github.com/strongdm/ai-cxdb/clients/go"
	cxdtypes "github.com/strongdm/ai-cxdb/clients/go/types"
)

// CXDBClient is the minimal interface for cxdb client operations.
// The real *cxdb.Client satisfies this interface.
type CXDBClient interface {
	CreateContext(ctx context.Context, baseTurnID uint64) (*cxdbclient.ContextHead, error)
	AppendTurn(ctx context.Context, req *cxdbclient.AppendRequest) (*cxdbclient.AppendResult, error)
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

	// Build the canonical ConversationItem payload.
	item := s.buildConversationItem(event, isOrphan)

	// Encode to msgpack using the official cxdb encoder.
	payload, err := cxdbclient.EncodeMsgpack(item)
	if err != nil {
		return fmt.Errorf("encode payload: %w", err)
	}

	// Append to context using canonical type identifiers.
	req := &cxdbclient.AppendRequest{
		ContextID:      contextID,
		ParentTurnID:   0,
		TypeID:         cxdtypes.TypeIDConversationItem,
		TypeVersion:    cxdtypes.TypeVersionConversationItem,
		Payload:        payload,
		IdempotencyKey: event.EventID,
	}

	_, err = s.client.AppendTurn(ctx, req)
	if err != nil {
		return fmt.Errorf("append turn: %w", err)
	}

	return nil
}

// buildConversationItem creates a canonical ConversationItem from an ErrorEvent.
func (s *cxdbSink) buildConversationItem(event aisen.ErrorEvent, isOrphan bool) *cxdtypes.ConversationItem {
	// Build title: "error_type: truncated_message"
	title := event.ErrorType
	if event.Message != "" {
		const maxMsgLen = 80
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

	item := &cxdtypes.ConversationItem{
		ItemType:  cxdtypes.ItemTypeSystem,
		Status:    cxdtypes.ItemStatusComplete,
		Timestamp: event.Timestamp.UnixMilli(),
		ID:        event.EventID,
		System: &cxdtypes.SystemMessage{
			Kind:    cxdtypes.SystemKindError,
			Title:   title,
			Content: buildErrorDetails(event),
		},
	}

	// Add context metadata for orphan contexts. cxdb expects this on the first turn.
	if isOrphan {
		item.ContextMetadata = &cxdtypes.ContextMetadata{
			Labels:    s.orphanLabels,
			ClientTag: s.clientTag,
		}
	}

	return item
}

// buildErrorDetails encodes the full ErrorEvent as JSON for SystemMessage.Content.
func buildErrorDetails(event aisen.ErrorEvent) string {
	details := map[string]any{
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
		details["system_state"] = map[string]any{
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

// Flush is a no-op for the cxdb sink (writes are synchronous).
func (s *cxdbSink) Flush(ctx context.Context) error {
	return nil
}

// Close is a no-op for the cxdb sink.
func (s *cxdbSink) Close() error {
	return nil
}
