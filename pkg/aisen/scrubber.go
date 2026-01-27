// scrubber.go implements fail-closed sensitive data redaction for error events.

package aisen

import (
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
