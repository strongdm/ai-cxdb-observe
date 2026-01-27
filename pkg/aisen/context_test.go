package aisen

import (
	"context"
	"testing"
)

func TestRunIDRoundTrip(t *testing.T) {
	ctx := context.Background()
	runID := "run-abc123"

	ctx = WithRunID(ctx, runID)
	got, ok := RunIDFromContext(ctx)

	if !ok {
		t.Error("RunIDFromContext returned ok=false, want ok=true")
	}
	if got != runID {
		t.Errorf("RunIDFromContext = %q, want %q", got, runID)
	}
}

func TestRunIDFromContext_NotSet(t *testing.T) {
	ctx := context.Background()

	got, ok := RunIDFromContext(ctx)

	if ok {
		t.Error("RunIDFromContext returned ok=true for empty context, want ok=false")
	}
	if got != "" {
		t.Errorf("RunIDFromContext = %q, want empty string", got)
	}
}

func TestRunIDFromContext_EmptyString(t *testing.T) {
	// Empty string should be treated as "not set"
	ctx := context.Background()
	ctx = WithRunID(ctx, "")

	_, ok := RunIDFromContext(ctx)

	if ok {
		t.Error("RunIDFromContext returned ok=true for empty run ID, want ok=false")
	}
}

func TestContextIDRoundTrip(t *testing.T) {
	ctx := context.Background()
	contextID := uint64(12345)

	ctx = WithContextID(ctx, contextID)
	got, ok := ContextIDFromContext(ctx)

	if !ok {
		t.Error("ContextIDFromContext returned ok=false, want ok=true")
	}
	if got != contextID {
		t.Errorf("ContextIDFromContext = %d, want %d", got, contextID)
	}
}

func TestContextIDFromContext_NotSet(t *testing.T) {
	ctx := context.Background()

	got, ok := ContextIDFromContext(ctx)

	if ok {
		t.Error("ContextIDFromContext returned ok=true for empty context, want ok=false")
	}
	if got != 0 {
		t.Errorf("ContextIDFromContext = %d, want 0", got)
	}
}

func TestContextIDFromContext_ZeroValue(t *testing.T) {
	// Zero is a valid context ID, should return ok=true
	ctx := context.Background()
	ctx = WithContextID(ctx, 0)

	got, ok := ContextIDFromContext(ctx)

	if !ok {
		t.Error("ContextIDFromContext returned ok=false for zero context ID, want ok=true")
	}
	if got != 0 {
		t.Errorf("ContextIDFromContext = %d, want 0", got)
	}
}

func TestContextPropagation_ChainedContexts(t *testing.T) {
	// Test that values propagate through context chain
	ctx := context.Background()
	ctx = WithRunID(ctx, "run-123")
	ctx = WithContextID(ctx, 456)

	// Create derived context
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	runID, ok := RunIDFromContext(ctx)
	if !ok || runID != "run-123" {
		t.Errorf("RunID not propagated through context chain")
	}

	contextID, ok := ContextIDFromContext(ctx)
	if !ok || contextID != 456 {
		t.Errorf("ContextID not propagated through context chain")
	}
}

func TestContextIDProvider_Interface(t *testing.T) {
	// Test that ContextIDProvider interface is defined correctly
	var _ ContextIDProvider = &mockContextIDProvider{}
}

type mockContextIDProvider struct {
	id  uint64
	err error
}

func (m *mockContextIDProvider) ContextID(ctx context.Context) (uint64, error) {
	return m.id, m.err
}
