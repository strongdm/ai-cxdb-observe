// event.go defines the canonical error event data structure for aisen.

package aisen

import "time"

// Severity indicates the severity level of an error event.
type Severity string

const (
	// SeverityWarning indicates a non-fatal issue that may need attention.
	SeverityWarning Severity = "warning"

	// SeverityError indicates a recoverable error that caused an operation to fail.
	SeverityError Severity = "error"

	// SeverityCrash indicates an unrecoverable error such as a panic.
	SeverityCrash Severity = "crash"
)

// SystemState captures system metrics at the time of an error.
type SystemState struct {
	// MemoryBytes is the current memory allocation in bytes.
	MemoryBytes int64

	// GoroutineCount is the number of active goroutines.
	GoroutineCount int

	// UptimeMs is the process uptime in milliseconds.
	UptimeMs int64

	// HostName is the hostname of the machine where the error occurred.
	HostName string
}

// ErrorEvent is the canonical error representation.
// All fields are populated by the collector before passing to sinks.
type ErrorEvent struct {
	// Identity fields

	// EventID is a unique identifier for this error event (UUID).
	EventID string

	// Timestamp is when the error occurred.
	Timestamp time.Time

	// Fingerprint is a hash for grouping similar errors.
	Fingerprint string

	// Error details

	// Severity indicates the error severity (warning, error, crash).
	Severity Severity

	// ErrorType categorizes the error (panic, error, timeout, oom, guardrail).
	ErrorType string

	// Message is the human-readable error message.
	Message string

	// StackTrace is the optional scrubbed stack trace.
	StackTrace string

	// Operation context

	// Operation indicates what was happening (tool, llm, guardrail, handoff).
	Operation string

	// OperationID is an optional identifier (e.g., tool call ID).
	OperationID string

	// AgentName is the name of the agent that was running.
	AgentName string

	// ToolName is the name of the tool that failed (if applicable).
	ToolName string

	// ToolArgs is the scrubbed JSON representation of tool arguments.
	ToolArgs string

	// Conversation context

	// ContextID is the optional cxdb context ID for linking to conversation.
	// Uses pointer to distinguish "not set" from "zero value".
	ContextID *uint64

	// TurnDepth is the optional turn number in the conversation.
	// Uses pointer to distinguish "not set" from "zero value".
	TurnDepth *int

	// System state

	// SystemState captures system metrics at error time.
	SystemState *SystemState

	// Impact

	// TokensWasted is the optional count of tokens consumed before failure.
	// Uses pointer to distinguish "not set" from "zero value".
	TokensWasted *int64

	// Arbitrary metadata

	// Metadata contains scrubbed key-value pairs for additional context.
	Metadata map[string]string
}
