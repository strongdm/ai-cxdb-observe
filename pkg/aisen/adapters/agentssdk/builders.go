// builders.go provides helper functions to build ErrorEvent from errors and panics.

package agentssdk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime/debug"

	"github.com/strongdm/ai-cxdb-observe/pkg/aisen"
)

// buildErrorEvent creates an ErrorEvent from an error with enrichment data.
func buildErrorEvent(err error, contextID uint64, enrichment Enrichment) aisen.ErrorEvent {
	event := aisen.ErrorEvent{
		Severity:    aisen.SeverityError,
		ErrorType:   classifyError(err),
		Message:     err.Error(),
		Operation:   enrichment.Operation,
		OperationID: enrichment.OperationID,
		AgentName:   enrichment.AgentName,
		ToolName:    enrichment.ToolName,
	}

	if contextID != 0 {
		event.ContextID = &contextID
	}

	// Attach operation history if available
	attachOperationHistory(&event, enrichment)

	return event
}

// buildPanicEvent creates an ErrorEvent from a recovered panic value.
func buildPanicEvent(recovered any, contextID uint64, enrichment Enrichment) aisen.ErrorEvent {
	event := aisen.ErrorEvent{
		Severity:    aisen.SeverityCrash,
		ErrorType:   "panic",
		Message:     formatRecovered(recovered),
		StackTrace:  string(debug.Stack()),
		Operation:   enrichment.Operation,
		OperationID: enrichment.OperationID,
		AgentName:   enrichment.AgentName,
		ToolName:    enrichment.ToolName,
	}

	if contextID != 0 {
		event.ContextID = &contextID
	}

	// Attach operation history if available
	attachOperationHistory(&event, enrichment)

	return event
}

// classifyError determines the error type based on the error.
func classifyError(err error) string {
	if err == nil {
		return "error"
	}

	// Check for context errors
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}

	// Check for guardrail-related errors (by error message pattern)
	// This is a heuristic; real implementation might use error types
	errMsg := err.Error()
	if containsGuardrailPattern(errMsg) {
		return "guardrail"
	}

	return "error"
}

// containsGuardrailPattern checks if an error message indicates a guardrail violation.
func containsGuardrailPattern(msg string) bool {
	patterns := []string{
		"guardrail",
		"content policy",
		"safety filter",
		"blocked by policy",
	}
	for _, p := range patterns {
		if containsIgnoreCase(msg, p) {
			return true
		}
	}
	return false
}

// containsIgnoreCase performs a case-insensitive substring check.
func containsIgnoreCase(s, substr string) bool {
	// Simple implementation - convert both to lowercase
	sl := make([]byte, len(s))
	subrl := make([]byte, len(substr))
	for i := 0; i < len(s); i++ {
		if s[i] >= 'A' && s[i] <= 'Z' {
			sl[i] = s[i] + 32
		} else {
			sl[i] = s[i]
		}
	}
	for i := 0; i < len(substr); i++ {
		if substr[i] >= 'A' && substr[i] <= 'Z' {
			subrl[i] = substr[i] + 32
		} else {
			subrl[i] = substr[i]
		}
	}

	// Check for substring
	sStr := string(sl)
	subStr := string(subrl)
	for i := 0; i <= len(sStr)-len(subStr); i++ {
		if sStr[i:i+len(subStr)] == subStr {
			return true
		}
	}
	return false
}

// formatRecovered formats a recovered panic value as a string.
func formatRecovered(recovered any) string {
	if recovered == nil {
		return "<nil>"
	}
	if err, ok := recovered.(error); ok {
		return err.Error()
	}
	return fmt.Sprintf("%v", recovered)
}

// attachOperationHistory attaches operation history from enrichment to the error event metadata.
// If no operations are recorded, the metadata key is omitted.
func attachOperationHistory(event *aisen.ErrorEvent, enrichment Enrichment) {
	history := enrichment.GetOperationHistory()

	// Skip if no operations recorded
	if len(history) == 0 {
		return
	}

	// Serialize to JSON
	historyJSON, err := json.Marshal(history)
	if err != nil {
		// On marshal error, don't attach history (fail gracefully)
		return
	}

	// Initialize metadata if needed
	if event.Metadata == nil {
		event.Metadata = make(map[string]string)
	}

	// Store as JSON string
	event.Metadata["aisen.operation_history_json"] = string(historyJSON)
}
