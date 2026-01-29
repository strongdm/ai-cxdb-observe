// Package stderr provides a sink that logs errors to stderr in human-readable format.
// Useful for development and debugging.
package stderr

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

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

	// Operation history (only in verbose mode)
	if s.verbose && event.Metadata != nil {
		if historyJSON, ok := event.Metadata["aisen.operation_history_json"]; ok {
			var history []map[string]any
			if err := json.Unmarshal([]byte(historyJSON), &history); err == nil && len(history) > 0 {
				fmt.Fprintf(os.Stderr, "        Operation History (%d operations):\n", len(history))
				for i, op := range history {
					s.formatOperation(os.Stderr, i+1, op)
				}
			}
		}
	}

	return nil
}

// formatOperation pretty-prints a single operation record.
func (s *stderrSink) formatOperation(w *os.File, index int, op map[string]any) {
	kind, _ := op["kind"].(string)
	agentName, _ := op["agent_name"].(string)

	// Parse timestamp if available
	var timestamp string
	if ts, ok := op["timestamp"].(string); ok {
		if t, err := time.Parse(time.RFC3339, ts); err == nil {
			timestamp = t.Format("15:04:05")
		}
	}

	// Format duration
	var durationStr string
	if durationMs, ok := op["duration_ms"].(float64); ok && durationMs > 0 {
		durationStr = fmt.Sprintf(" (%s)", formatDuration(int64(durationMs)))
	}

	// Header line
	fmt.Fprintf(w, "          %d. [%s]", index, kind)
	if timestamp != "" {
		fmt.Fprintf(w, " %s", timestamp)
	}
	if agentName != "" {
		fmt.Fprintf(w, " agent=%s", agentName)
	}
	fmt.Fprintf(w, "%s\n", durationStr)

	// LLM-specific details
	if kind == "llm" {
		if llm, ok := op["llm"].(map[string]any); ok {
			if model, ok := llm["model"].(string); ok {
				fmt.Fprintf(w, "             Model: %s", model)
				if provider, ok := llm["provider"].(string); ok {
					fmt.Fprintf(w, " (%s)", provider)
				}
				fmt.Fprintln(w)
			}
			if promptTokens, ok := llm["prompt_tokens"].(float64); ok {
				completionTokens, _ := llm["completion_tokens"].(float64)
				fmt.Fprintf(w, "             Tokens: %d prompt + %d completion = %d total\n",
					int(promptTokens), int(completionTokens), int(promptTokens+completionTokens))
			}
			if finishReason, ok := llm["finish_reason"].(string); ok && finishReason != "" {
				fmt.Fprintf(w, "             Finish: %s\n", finishReason)
			}
		}
	}

	// Tool-specific details
	if kind == "tool" {
		if tool, ok := op["tool"].(map[string]any); ok {
			if name, ok := tool["name"].(string); ok {
				fmt.Fprintf(w, "             Tool: %s", name)
				if callID, ok := tool["call_id"].(string); ok {
					fmt.Fprintf(w, " (id: %s)", callID)
				}
				fmt.Fprintln(w)
			}
			if inputSize, ok := tool["input_size"].(float64); ok {
				outputSize, _ := tool["output_size"].(float64)
				fmt.Fprintf(w, "             I/O: %d bytes in, %d bytes out\n",
					int(inputSize), int(outputSize))
			}
		}
	}

	// Error field
	if errMsg, ok := op["error"].(string); ok && errMsg != "" {
		fmt.Fprintf(w, "             Error: %s\n", errMsg)
	}
}

// formatDuration formats milliseconds as a human-readable duration.
func formatDuration(ms int64) string {
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	return fmt.Sprintf("%.3fs", float64(ms)/1000.0)
}

// Flush is a no-op for stderr sink.
func (s *stderrSink) Flush(ctx context.Context) error {
	return nil
}

// Close is a no-op for stderr sink.
func (s *stderrSink) Close() error {
	return nil
}
