// fingerprint.go generates stable hashes for grouping similar errors.

package aisen

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
)

// Fingerprint generates a hash for grouping similar errors.
// The fingerprint is based on:
//   - error_type, operation, agent_name, tool_name
//   - First 3 stack frames (function names only, normalized)
//
// It ignores variable data like timestamps, event IDs, messages,
// line numbers, and memory addresses.
func Fingerprint(event ErrorEvent) string {
	// Build the fingerprint input from stable fields
	var parts []string
	parts = append(parts, event.ErrorType)
	parts = append(parts, event.Operation)
	parts = append(parts, event.AgentName)
	parts = append(parts, event.ToolName)

	// Add normalized stack frames
	frames := normalizeStackTrace(event.StackTrace)
	parts = append(parts, frames...)

	// Join and hash
	input := strings.Join(parts, "|")
	hash := sha256.Sum256([]byte(input))

	// Return hex-encoded first 16 bytes (32 hex chars)
	return hex.EncodeToString(hash[:16])
}

// Regex patterns for stack trace parsing
var (
	// Match function names like "main.doSomething" or "pkg/subpkg.Function"
	funcNamePattern = regexp.MustCompile(`^([a-zA-Z0-9_./]+\.[a-zA-Z0-9_]+)`)

	// Match line numbers like ":42" or ":123"
	lineNumPattern = regexp.MustCompile(`:\d+`)

	// Match memory addresses like "0x1234abcd"
	memAddrPattern = regexp.MustCompile(`0x[0-9a-fA-F]+`)

	// Match offset patterns like "+0x123"
	offsetPattern = regexp.MustCompile(`\+0x[0-9a-fA-F]+`)
)

// normalizeStackTrace extracts the first 3 function names from a stack trace,
// stripping line numbers, memory addresses, and other variable data.
func normalizeStackTrace(trace string) []string {
	if trace == "" {
		return nil
	}

	var frames []string
	lines := strings.Split(trace, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Skip header lines like "goroutine 1 [running]:"
		if strings.HasPrefix(line, "goroutine ") {
			continue
		}

		// Skip file path lines (start with tab or contain .go:)
		if strings.HasPrefix(line, "\t") || strings.HasPrefix(line, "/") {
			continue
		}

		// Extract function name
		// Line looks like: "main.doSomething(0x1234)" or "pkg.Function()"
		funcLine := line

		// Remove memory addresses and offsets
		funcLine = memAddrPattern.ReplaceAllString(funcLine, "")
		funcLine = offsetPattern.ReplaceAllString(funcLine, "")

		// Remove parentheses and arguments
		if idx := strings.Index(funcLine, "("); idx > 0 {
			funcLine = funcLine[:idx]
		}

		funcLine = strings.TrimSpace(funcLine)
		if funcLine == "" {
			continue
		}

		// Validate it looks like a function name
		if match := funcNamePattern.FindString(funcLine); match != "" {
			frames = append(frames, match)
			if len(frames) >= 3 {
				break
			}
		}
	}

	return frames
}
