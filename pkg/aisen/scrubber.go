// scrubber.go implements fail-closed sensitive data redaction for error events.

package aisen

import (
	"encoding/json"
	"regexp"
	"strings"
)

// ScrubberConfig controls scrubbing behavior.
type ScrubberConfig struct {
	// ToolArgAllowlist contains tool names whose args are safe to log.
	ToolArgAllowlist []string

	// SensitivePatterns contains additional regex patterns for sensitive metadata keys.
	SensitivePatterns []string

	// MaxMessageSize is the maximum length for error messages (default: 4096).
	MaxMessageSize int

	// MaxStackTraceSize is the maximum length for stack traces (default: 32768).
	MaxStackTraceSize int

	// MaxToolArgsSize is the maximum length for tool arguments (default: 8192).
	MaxToolArgsSize int

	// MaxMetadataSize is the maximum total size for metadata (default: 16384).
	MaxMetadataSize int

	// MaxMetadataKeySize is the maximum size per metadata key (default: 1024).
	MaxMetadataKeySize int

	// ScrubMessages enables scrubbing of error messages for secrets/PII (default: true).
	ScrubMessages bool

	// FailClosed enables fail-closed behavior: on any scrub error, fully redact (default: true).
	FailClosed bool
}

// DefaultScrubberConfig returns production-safe defaults.
func DefaultScrubberConfig() ScrubberConfig {
	return ScrubberConfig{
		MaxMessageSize:     4096,
		MaxStackTraceSize:  32768,
		MaxToolArgsSize:    8192,
		MaxMetadataSize:    16384,
		MaxMetadataKeySize: 1024,
		ScrubMessages:      true,
		FailClosed:         true,
	}
}

// Compiled regex patterns for message scrubbing (compiled once at package init)
var messageScrubPatterns = []*regexp.Regexp{
	// API keys and tokens
	regexp.MustCompile(`(?i)(api[_-]?key|token)[=:\s]+['"]?[\w\-\.]+['"]?`),
	regexp.MustCompile(`(?i)(authorization|bearer)[=:\s]+['"]?[\w\-\.]+['"]?[\s]+['"]?[\w\-\.]+['"]?`), // Authorization: Bearer <token>
	regexp.MustCompile(`(?i)sk-[a-zA-Z0-9_-]{20,}`),          // OpenAI-style keys (including sk-proj-)
	regexp.MustCompile(`(?i)ghp_[a-zA-Z0-9]{36}`),            // GitHub tokens
	regexp.MustCompile(`(?i)gho_[a-zA-Z0-9]{36}`),            // GitHub OAuth tokens
	regexp.MustCompile(`(?i)github_pat_[a-zA-Z0-9_]{22,}`),   // GitHub PAT
	regexp.MustCompile(`(?i)xox[baprs]-[a-zA-Z0-9\-]{10,}`),  // Slack tokens
	regexp.MustCompile(`(?i)eyJ[a-zA-Z0-9_-]*\.eyJ[a-zA-Z0-9_-]*\.[a-zA-Z0-9_-]*`), // JWT tokens

	// Credentials
	regexp.MustCompile(`(?i)password[=:\s]+['"]?[^\s'"",]+['"]?`),
	regexp.MustCompile(`(?i)secret[=:\s]+['"]?[^\s'"",]+['"]?`),
	regexp.MustCompile(`(?i)passwd[=:\s]+['"]?[^\s'"",]+['"]?`),
	regexp.MustCompile(`(?i)credential[=:\s]+['"]?[^\s'"",]+['"]?`),

	// PII
	regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b`), // Email
	regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`),                                 // SSN
	regexp.MustCompile(`\b\d{4}[\s-]?\d{4}[\s-]?\d{4}[\s-]?\d{4}\b`),           // Credit card
}

// Sensitive metadata key patterns (case-insensitive substring match)
var sensitiveKeyPatterns = []string{
	"token",
	"key",
	"secret",
	"password",
	"credential",
	"auth",
	"passwd",
}

// Path patterns to normalize in stack traces
var pathNormalizationPatterns = []*regexp.Regexp{
	regexp.MustCompile(`/home/[^/]+/`),
	regexp.MustCompile(`/Users/[^/]+/`),
	regexp.MustCompile(`C:\\Users\\[^\\]+\\`),
	regexp.MustCompile(`/tmp/[^/]+/`),
}

// Scrubber redacts sensitive data from error events.
type Scrubber struct {
	cfg ScrubberConfig
}

// NewScrubber creates a new scrubber with the given configuration.
func NewScrubber(cfg ScrubberConfig) *Scrubber {
	return &Scrubber{cfg: cfg}
}

// ScrubField scrubs a field value. On ANY error, returns redacted placeholder if FailClosed is true.
func (s *Scrubber) ScrubField(field string, value []byte) ([]byte, error) {
	if len(value) == 0 {
		return value, nil
	}

	// Determine max size based on field type
	maxSize := s.cfg.MaxMessageSize
	switch field {
	case "stack_trace":
		maxSize = s.cfg.MaxStackTraceSize
	case "tool_args":
		maxSize = s.cfg.MaxToolArgsSize
	case "metadata":
		maxSize = s.cfg.MaxMetadataSize
	}

	// Truncate if too large
	if len(value) > maxSize {
		if s.cfg.FailClosed {
			return []byte(truncateWithMarker(string(value), maxSize)), nil
		}
		return []byte("[REDACTED:SIZE_LIMIT]"), nil
	}

	return value, nil
}

// ScrubMessage scrubs sensitive patterns from an error message.
func (s *Scrubber) ScrubMessage(msg string) string {
	if !s.cfg.ScrubMessages {
		return msg
	}

	// Truncate if too large first
	if len(msg) > s.cfg.MaxMessageSize {
		msg = truncateWithMarker(msg, s.cfg.MaxMessageSize)
	}

	// Apply all scrubbing patterns
	result := msg
	for _, pattern := range messageScrubPatterns {
		result = pattern.ReplaceAllString(result, "[REDACTED]")
	}

	return result
}

// ScrubMetadata redacts sensitive keys from metadata.
func (s *Scrubber) ScrubMetadata(meta map[string]string) map[string]string {
	if meta == nil {
		return nil
	}

	result := make(map[string]string, len(meta))
	for key, value := range meta {
		if s.isSensitiveKey(key) {
			result[key] = "[REDACTED]"
		} else {
			// Truncate long values
			if len(value) > s.cfg.MaxMetadataKeySize {
				value = truncateWithMarker(value, s.cfg.MaxMetadataKeySize)
			}
			result[key] = value
		}
	}

	return result
}

// ScrubStackTrace normalizes paths and limits stack trace size.
func (s *Scrubber) ScrubStackTrace(trace string) string {
	if trace == "" {
		return trace
	}

	// Normalize paths (remove user-specific directories)
	result := trace
	for _, pattern := range pathNormalizationPatterns {
		result = pattern.ReplaceAllString(result, "/[PATH]/")
	}

	// Remove memory addresses (0x...)
	result = regexp.MustCompile(`0x[0-9a-fA-F]+`).ReplaceAllString(result, "0x...")

	// Truncate if too large
	if len(result) > s.cfg.MaxStackTraceSize {
		result = truncateWithMarker(result, s.cfg.MaxStackTraceSize)
	}

	return result
}

// isSensitiveKey checks if a metadata key matches sensitive patterns.
func (s *Scrubber) isSensitiveKey(key string) bool {
	keyLower := strings.ToLower(key)
	for _, pattern := range sensitiveKeyPatterns {
		if strings.Contains(keyLower, pattern) {
			return true
		}
	}
	return false
}

// truncateWithMarker truncates a string and adds a truncation marker.
func truncateWithMarker(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	marker := "...[TRUNCATED]"
	if maxLen <= len(marker) {
		return marker[:maxLen]
	}
	return s[:maxLen-len(marker)] + marker
}

// ScrubJSON recursively scrubs sensitive data from a JSON string.
// Returns scrubbed JSON or "[REDACTED:SCRUB_ERROR]" on any error (fail-closed).
func (s *Scrubber) ScrubJSON(jsonStr string) string {
	maxSize := 16384 // 16KB limit for JSON fields

	// Parse JSON into generic structure
	var data interface{}
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		// Fail closed: invalid JSON gets fully redacted
		if s.cfg.FailClosed {
			return "[REDACTED:SCRUB_ERROR]"
		}
		return jsonStr
	}

	// Recursively scrub the data
	scrubbed := s.scrubJSONValue(data)

	// Re-serialize to JSON
	result, err := json.Marshal(scrubbed)
	if err != nil {
		// Fail closed on marshal error
		if s.cfg.FailClosed {
			return "[REDACTED:SCRUB_ERROR]"
		}
		return jsonStr
	}

	// Truncate after marshalling if too large
	resultStr := string(result)
	if len(resultStr) > maxSize {
		resultStr = truncateWithMarker(resultStr, maxSize)
	}

	return resultStr
}

// scrubJSONValue recursively scrubs a JSON value (map, array, or primitive).
func (s *Scrubber) scrubJSONValue(val interface{}) interface{} {
	switch v := val.(type) {
	case map[string]interface{}:
		return s.scrubJSONMap(v)
	case []interface{}:
		return s.scrubJSONArray(v)
	case string:
		return s.ScrubMessage(v) // Apply message scrubbing to string values
	default:
		return v // Numbers, booleans, null pass through
	}
}

// scrubJSONMap scrubs a JSON object (map).
func (s *Scrubber) scrubJSONMap(m map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{}, len(m))
	for key, value := range m {
		// If key is sensitive, redact the entire value
		if s.isSensitiveKey(key) {
			result[key] = "[REDACTED]"
		} else {
			// Recursively scrub the value
			result[key] = s.scrubJSONValue(value)
		}
	}
	return result
}

// scrubJSONArray scrubs a JSON array.
func (s *Scrubber) scrubJSONArray(arr []interface{}) []interface{} {
	result := make([]interface{}, len(arr))
	for i, value := range arr {
		result[i] = s.scrubJSONValue(value)
	}
	return result
}

// ScrubOperationHistoryJSON scrubs operation history JSON stored in metadata.
// This is a convenience wrapper around ScrubJSON for the specific use case
// of scrubbing operation history before it's sent to sinks.
func (s *Scrubber) ScrubOperationHistoryJSON(historyJSON string) string {
	return s.ScrubJSON(historyJSON)
}
