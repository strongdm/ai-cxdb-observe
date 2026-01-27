package cxdb

import (
	"context"
	"strings"
	"testing"

	"github.com/strongdm/ai-cxdb-observe/pkg/aisen"
	cxdtypes "github.com/strongdm/ai-cxdb/clients/go/types"
)

func TestE2E_CollectorToCXDB_CanonicalAndScrubbed(t *testing.T) {
	client := &mockCXDBClient{}
	sink := NewCXDBSink(client)
	collector := aisen.NewCollector(
		aisen.WithSink(sink),
		aisen.WithDefaultScrubbing(),
	)

	err := collector.Record(context.Background(), aisen.ErrorEvent{
		Severity:  aisen.SeverityError,
		ErrorType: "tool",
		Message:   "api_key=sk-verysecret user@example.com",
		Operation: "tool",
		Metadata: map[string]string{
			"auth_token": "secret-token",
			"safe_label": "safe-value",
		},
	})
	if err != nil {
		t.Fatalf("Record returned error: %v", err)
	}

	createCalls := client.getCreateContextCalls()
	if len(createCalls) != 1 {
		t.Fatalf("expected orphan context creation, got %d calls", len(createCalls))
	}

	appendReqs := client.getAppendRequests()
	if len(appendReqs) != 1 {
		t.Fatalf("expected 1 append request, got %d", len(appendReqs))
	}

	req := appendReqs[0]
	if req.IdempotencyKey == "" {
		t.Fatalf("IdempotencyKey should be set from generated event ID")
	}

	item := decodeConversationItem(t, req.Payload)
	if item.ItemType != cxdtypes.ItemTypeSystem {
		t.Fatalf("ItemType = %q, want %q", item.ItemType, cxdtypes.ItemTypeSystem)
	}
	if item.System == nil || item.System.Kind != cxdtypes.SystemKindError {
		t.Fatalf("System kind should be error, got %+v", item.System)
	}
	if item.ID == "" || item.ID != req.IdempotencyKey {
		t.Fatalf("item.ID should match idempotency key, got %q vs %q", item.ID, req.IdempotencyKey)
	}

	details := decodeDetailsJSON(t, item.System.Content)
	message, _ := details["message"].(string)
	if strings.Contains(message, "sk-verysecret") || strings.Contains(message, "user@example.com") {
		t.Fatalf("message should be scrubbed, got %q", message)
	}
	if fp, _ := details["fingerprint"].(string); fp == "" {
		t.Fatalf("fingerprint should be populated in persisted details")
	}

	meta, _ := details["metadata"].(map[string]any)
	if meta == nil {
		t.Fatalf("metadata should be present in persisted details")
	}
	if meta["auth_token"] != "[REDACTED]" {
		t.Fatalf("auth_token should be redacted, got %v", meta["auth_token"])
	}
	if meta["safe_label"] != "safe-value" {
		t.Fatalf("safe_label should be preserved, got %v", meta["safe_label"])
	}

	if item.ContextMetadata == nil {
		t.Fatalf("orphan contexts should include context metadata")
	}
	if len(item.ContextMetadata.Labels) == 0 {
		t.Fatalf("orphan context labels should be set")
	}
}
