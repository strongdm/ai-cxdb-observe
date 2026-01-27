// context.go provides utilities for propagating run IDs and cxdb context IDs
// through Go context.Context.

package aisen

import "context"

// Context key types (unexported to avoid collisions)
type runIDKey struct{}
type contextIDKey struct{}

// contextIDSet is used to distinguish "zero value" from "not set"
type contextIDSet struct {
	id uint64
}

// WithRunID returns a context with the run ID attached.
// The run ID is used to correlate hook enrichment with runner-boundary errors.
func WithRunID(ctx context.Context, runID string) context.Context {
	return context.WithValue(ctx, runIDKey{}, runID)
}

// RunIDFromContext extracts the run ID from context.
// Returns empty string and false if not set or if the run ID is empty.
func RunIDFromContext(ctx context.Context) (string, bool) {
	v := ctx.Value(runIDKey{})
	id, ok := v.(string)
	return id, ok && id != ""
}

// WithContextID returns a context with the cxdb context ID attached.
// This allows errors to be linked to conversation context.
func WithContextID(ctx context.Context, contextID uint64) context.Context {
	return context.WithValue(ctx, contextIDKey{}, contextIDSet{id: contextID})
}

// ContextIDFromContext extracts the cxdb context ID from context.
// Returns 0 and false if not set.
func ContextIDFromContext(ctx context.Context) (uint64, bool) {
	v := ctx.Value(contextIDKey{})
	if v == nil {
		return 0, false
	}
	set, ok := v.(contextIDSet)
	if !ok {
		return 0, false
	}
	return set.id, true
}

// ContextIDProvider is an optional interface that session implementations can
// satisfy to enable automatic context linkage for error events.
//
// The ai-agents-sdk CXDBSession already implements this interface via its
// ContextID() method.
type ContextIDProvider interface {
	ContextID(ctx context.Context) (uint64, error)
}
