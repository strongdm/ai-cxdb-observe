// Package stderr provides a sink that logs errors to stderr in human-readable format.
// Useful for development and debugging.
package stderr

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/strongdm/ai-cxdb-observe/pkg/aisen"
)

// StderrSinkOption configures the stderr sink.
type StderrSinkOption func(*stderrSinkConfig)

type stderrSinkConfig struct {
	verbose bool
}

// WithVerbose enables full error details including stack traces.
func WithVerbose() StderrSinkOption {
	return func(c *stderrSinkConfig) {
		c.verbose = true
	}
}

// stderrSink writes errors to stderr in human-readable format.
type stderrSink struct {
	verbose bool
}

// NewStderrSink creates a sink that writes to stderr.
func NewStderrSink(opts ...StderrSinkOption) aisen.Sink {
	cfg := &stderrSinkConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	return &stderrSink{
		verbose: cfg.verbose,
	}
}

// Write formats and outputs the error event to stderr.
func (s *stderrSink) Write(ctx context.Context, event aisen.ErrorEvent) error {
	// Format severity as uppercase
	severity := strings.ToUpper(string(event.Severity))

	// Build the main line
	// Format: [AISEN] <timestamp> <SEVERITY> <error_type> in <operation> <tool_name> (agent: <agent_name>)
	timestamp := event.Timestamp.Format("2006-01-02T15:04:05Z07:00")

	var parts []string
	parts = append(parts, fmt.Sprintf("[AISEN] %s %s %s", timestamp, severity, event.ErrorType))

	if event.Operation != "" {
		parts = append(parts, fmt.Sprintf("in %s", event.Operation))
	}
	if event.ToolName != "" {
		parts = append(parts, event.ToolName)
	}
	if event.AgentName != "" {
		parts = append(parts, fmt.Sprintf("(agent: %s)", event.AgentName))
	}

	fmt.Fprintln(os.Stderr, strings.Join(parts, " "))

	// Message line
	if event.Message != "" {
		fmt.Fprintf(os.Stderr, "        Message: %s\n", event.Message)
	}

	// Fingerprint line
	if event.Fingerprint != "" {
		fmt.Fprintf(os.Stderr, "        Fingerprint: %s\n", event.Fingerprint)
	}

	// Context line (if available)
	if event.ContextID != nil {
		if event.TurnDepth != nil {
			fmt.Fprintf(os.Stderr, "        Context: %d (turn %d)\n", *event.ContextID, *event.TurnDepth)
		} else {
			fmt.Fprintf(os.Stderr, "        Context: %d\n", *event.ContextID)
		}
	}

	// Stack trace (only in verbose mode)
	if s.verbose && event.StackTrace != "" {
		fmt.Fprintf(os.Stderr, "        Stack trace:\n")
		for _, line := range strings.Split(event.StackTrace, "\n") {
			fmt.Fprintf(os.Stderr, "          %s\n", line)
		}
	}

	return nil
}

// Flush is a no-op for stderr sink.
func (s *stderrSink) Flush(ctx context.Context) error {
	return nil
}

// Close is a no-op for stderr sink.
func (s *stderrSink) Close() error {
	return nil
}
