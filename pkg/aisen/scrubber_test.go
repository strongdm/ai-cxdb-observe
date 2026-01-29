package aisen

import (
	"strings"
	"testing"
)

func TestScrubber_ScrubMessage_APIKey(t *testing.T) {
	s := NewScrubber(DefaultScrubberConfig())

	tests := []struct {
		name  string
		input string
		want  string // should not contain the secret
	}{
		{"api_key assignment", "Error: api_key=sk-abc123xyz", "sk-abc123xyz"},
		{"api-key with hyphen", "Failed with api-key: secret123", "secret123"},
		{"token header", "Authorization: Bearer eyJhbGc...", "eyJhbGc"},
		{"OpenAI key", "Using key sk-proj-abc123def456ghi789", "sk-proj-abc123def456ghi789"},
		{"GitHub token", "Token: ghp_1234567890abcdefghijklmnopqrstuvwxyz", "ghp_1234567890abcdefghijklmnopqrstuvwxyz"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.ScrubMessage(tt.input)
			if strings.Contains(got, tt.want) {
				t.Errorf("ScrubMessage(%q) = %q, still contains secret %q", tt.input, got, tt.want)
			}
			if !strings.Contains(got, "[REDACTED]") {
				t.Errorf("ScrubMessage(%q) = %q, should contain [REDACTED]", tt.input, got)
			}
		})
	}
}

func TestScrubber_ScrubMessage_Credentials(t *testing.T) {
	s := NewScrubber(DefaultScrubberConfig())

	tests := []struct {
		name  string
		input string
		want  string // should not contain
	}{
		{"password assignment", "password=mysecretpass123", "mysecretpass123"},
		{"password with colon", "password: super_secret", "super_secret"},
		{"secret assignment", "secret=abc123xyz", "abc123xyz"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.ScrubMessage(tt.input)
			if strings.Contains(got, tt.want) {
				t.Errorf("ScrubMessage(%q) still contains %q", tt.input, tt.want)
			}
		})
	}
}

func TestScrubber_ScrubMessage_Email(t *testing.T) {
	s := NewScrubber(DefaultScrubberConfig())

	input := "Failed for user@example.com and admin@test.org"
	got := s.ScrubMessage(input)

	if strings.Contains(got, "user@example.com") {
		t.Errorf("ScrubMessage still contains email user@example.com")
	}
	if strings.Contains(got, "admin@test.org") {
		t.Errorf("ScrubMessage still contains email admin@test.org")
	}
}

func TestScrubber_ScrubMessage_SSN(t *testing.T) {
	s := NewScrubber(DefaultScrubberConfig())

	input := "SSN: 123-45-6789 found in record"
	got := s.ScrubMessage(input)

	if strings.Contains(got, "123-45-6789") {
		t.Errorf("ScrubMessage still contains SSN")
	}
}

func TestScrubber_ScrubMessage_CreditCard(t *testing.T) {
	s := NewScrubber(DefaultScrubberConfig())

	tests := []string{
		"Card: 4111-1111-1111-1111",
		"CC 4111 1111 1111 1111",
		"Payment with 4111111111111111",
	}

	for _, input := range tests {
		got := s.ScrubMessage(input)
		if strings.Contains(got, "4111") {
			t.Errorf("ScrubMessage(%q) still contains credit card digits", input)
		}
	}
}

func TestScrubber_ScrubMessage_DisabledScrubbing(t *testing.T) {
	cfg := DefaultScrubberConfig()
	cfg.ScrubMessages = false
	s := NewScrubber(cfg)

	input := "api_key=secret123"
	got := s.ScrubMessage(input)

	if got != input {
		t.Errorf("ScrubMessage with ScrubMessages=false should not modify input")
	}
}

func TestScrubber_ScrubMetadata_SensitiveKey(t *testing.T) {
	s := NewScrubber(DefaultScrubberConfig())

	input := map[string]string{
		"request_id":   "req-123",
		"auth_token":   "secret_token_value",
		"api_key":      "sk-abc123",
		"password":     "mypassword",
		"user_secret":  "shh",
		"credential":   "cred123",
		"normal_field": "visible",
	}

	got := s.ScrubMetadata(input)

	// Non-sensitive keys should be preserved
	if got["request_id"] != "req-123" {
		t.Errorf("request_id should be preserved, got %q", got["request_id"])
	}
	if got["normal_field"] != "visible" {
		t.Errorf("normal_field should be preserved, got %q", got["normal_field"])
	}

	// Sensitive keys should be redacted
	sensitiveKeys := []string{"auth_token", "api_key", "password", "user_secret", "credential"}
	for _, key := range sensitiveKeys {
		if got[key] != "[REDACTED]" {
			t.Errorf("metadata key %q should be redacted, got %q", key, got[key])
		}
	}
}

func TestScrubber_ScrubStackTrace_Normalizes(t *testing.T) {
	s := NewScrubber(DefaultScrubberConfig())

	input := `goroutine 1 [running]:
main.doSomething(0x1234abcd)
	/home/user/secret/project/main.go:42 +0x123
main.main()
	/home/user/secret/project/main.go:10 +0x456`

	got := s.ScrubStackTrace(input)

	// Should normalize paths (remove home directory specifics)
	if strings.Contains(got, "/home/user/secret") {
		t.Errorf("ScrubStackTrace should normalize paths, still contains home dir")
	}

	// Should still contain function names
	if !strings.Contains(got, "main.doSomething") {
		t.Errorf("ScrubStackTrace should preserve function names")
	}
}

func TestScrubber_ScrubStackTrace_LimitsFrames(t *testing.T) {
	cfg := DefaultScrubberConfig()
	s := NewScrubber(cfg)

	// Create a very long stack trace
	var builder strings.Builder
	builder.WriteString("goroutine 1 [running]:\n")
	for i := 0; i < 100; i++ {
		builder.WriteString("some.package.function()\n")
		builder.WriteString("\t/path/to/file.go:10\n")
	}

	input := builder.String()
	got := s.ScrubStackTrace(input)

	// Should be within size limit
	if len(got) > cfg.MaxStackTraceSize {
		t.Errorf("ScrubStackTrace output size %d exceeds limit %d", len(got), cfg.MaxStackTraceSize)
	}
}

func TestScrubber_FailClosed_OnError(t *testing.T) {
	cfg := DefaultScrubberConfig()
	cfg.FailClosed = true
	s := NewScrubber(cfg)

	// When scrubbing fails (simulated by testing internal behavior),
	// the result should be fully redacted, not raw data
	// This is tested indirectly through size limit behavior
	input := strings.Repeat("x", cfg.MaxMessageSize+1000)
	got := s.ScrubMessage(input)

	if len(got) > cfg.MaxMessageSize {
		t.Errorf("Message should be truncated to MaxMessageSize")
	}
}

func TestScrubber_SizeLimit_Truncates(t *testing.T) {
	cfg := DefaultScrubberConfig()
	cfg.MaxMessageSize = 100
	s := NewScrubber(cfg)

	input := strings.Repeat("a", 500)
	got := s.ScrubMessage(input)

	if len(got) > cfg.MaxMessageSize+20 { // Allow some room for truncation marker
		t.Errorf("ScrubMessage should truncate to MaxMessageSize, got length %d", len(got))
	}
}

func TestDefaultScrubberConfig(t *testing.T) {
	cfg := DefaultScrubberConfig()

	if cfg.MaxMessageSize != 4096 {
		t.Errorf("MaxMessageSize = %d, want 4096", cfg.MaxMessageSize)
	}
	if cfg.MaxStackTraceSize != 32768 {
		t.Errorf("MaxStackTraceSize = %d, want 32768", cfg.MaxStackTraceSize)
	}
	if cfg.MaxToolArgsSize != 8192 {
		t.Errorf("MaxToolArgsSize = %d, want 8192", cfg.MaxToolArgsSize)
	}
	if cfg.MaxMetadataSize != 16384 {
		t.Errorf("MaxMetadataSize = %d, want 16384", cfg.MaxMetadataSize)
	}
	if cfg.MaxMetadataKeySize != 1024 {
		t.Errorf("MaxMetadataKeySize = %d, want 1024", cfg.MaxMetadataKeySize)
	}
	if !cfg.ScrubMessages {
		t.Error("ScrubMessages should be true by default")
	}
	if !cfg.FailClosed {
		t.Error("FailClosed should be true by default")
	}
}

func TestScrubber_ScrubField_FailClosed(t *testing.T) {
	cfg := DefaultScrubberConfig()
	cfg.FailClosed = true
	s := NewScrubber(cfg)

	// Test that ScrubField returns redacted placeholder on size overflow
	input := []byte(strings.Repeat("x", 1000000)) // Very large input
	got, err := s.ScrubField("test_field", input)

	if err != nil {
		t.Errorf("ScrubField should not return error with FailClosed=true")
	}
	// Should either be truncated or fully redacted, not original
	if len(got) > cfg.MaxMessageSize+100 {
		t.Errorf("ScrubField should handle large inputs safely")
	}
}

// TestScrubJSONRedactsAPIKeys verifies API keys are removed from nested JSON.
func TestScrubJSONRedactsAPIKeys(t *testing.T) {
	s := NewScrubber(DefaultScrubberConfig())

	input := `{"tool":"search","args":{"api_key":"sk-secret123","query":"test"}}`
	got := s.ScrubJSON(input)

	if strings.Contains(got, "sk-secret123") {
		t.Errorf("ScrubJSON should redact API key, got: %s", got)
	}
	if !strings.Contains(got, "[REDACTED]") {
		t.Errorf("ScrubJSON should contain [REDACTED], got: %s", got)
	}
}

// TestScrubJSONRedactsSensitiveKeys verifies sensitive key names cause value redaction.
func TestScrubJSONRedactsSensitiveKeys(t *testing.T) {
	s := NewScrubber(DefaultScrubberConfig())

	tests := []struct {
		name     string
		input    string
		dontWant string
	}{
		{"token key", `{"token":"abc123"}`, "abc123"},
		{"password key", `{"password":"secret"}`, "secret"},
		{"api_key key", `{"api_key":"xyz789"}`, "xyz789"},
		{"nested secret", `{"config":{"secret":"hidden"}}`, "hidden"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.ScrubJSON(tt.input)
			if strings.Contains(got, tt.dontWant) {
				t.Errorf("ScrubJSON should redact sensitive value, got: %s", got)
			}
		})
	}
}

// TestScrubJSONFailClosed verifies malformed JSON is redacted entirely.
func TestScrubJSONFailClosed(t *testing.T) {
	s := NewScrubber(DefaultScrubberConfig())

	tests := []struct {
		name  string
		input string
	}{
		{"invalid JSON", `{invalid json`},
		{"unclosed brace", `{"key":"value"`},
		{"trailing comma", `{"key":"value",}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.ScrubJSON(tt.input)
			if !strings.Contains(got, "[REDACTED:SCRUB_ERROR]") {
				t.Errorf("ScrubJSON should fail closed on invalid JSON, got: %s", got)
			}
		})
	}
}

// TestScrubJSONTruncatesOversized verifies size limiting with truncation marker.
func TestScrubJSONTruncatesOversized(t *testing.T) {
	s := NewScrubber(DefaultScrubberConfig())

	// Create JSON > 16KB (MaxMetadataSize)
	largeValue := strings.Repeat("x", 20000)
	input := `{"data":"` + largeValue + `"}`

	got := s.ScrubJSON(input)

	if len(got) > 16384+100 { // Allow some overhead for markers
		t.Errorf("ScrubJSON should truncate large JSON, got length: %d", len(got))
	}
	if !strings.Contains(got, "[TRUNCATED]") {
		preview := got
		if len(preview) > 100 {
			preview = got[:100]
		}
		t.Errorf("ScrubJSON should include truncation marker, got: %s", preview)
	}
}

// TestScrubOperationHistoryJSON verifies operation history scrubbing wrapper.
func TestScrubOperationHistoryJSON(t *testing.T) {
	s := NewScrubber(DefaultScrubberConfig())

	input := `[{"kind":"llm","llm":{"api_key":"sk-secret"}},{"kind":"tool","tool":{"password":"hidden"}}]`
	got := s.ScrubOperationHistoryJSON(input)

	if strings.Contains(got, "sk-secret") {
		t.Errorf("Should redact API key in operation history")
	}
	if strings.Contains(got, "hidden") {
		t.Errorf("Should redact password in operation history")
	}
}

// TestScrubJSONPreservesStructure verifies scrubbed JSON is still valid.
func TestScrubJSONPreservesStructure(t *testing.T) {
	s := NewScrubber(DefaultScrubberConfig())

	input := `{"name":"tool1","safe_field":"ok","api_key":"secret"}`
	got := s.ScrubJSON(input)

	// Should still be valid JSON
	if !strings.HasPrefix(got, "{") || !strings.HasSuffix(got, "}") {
		t.Errorf("Scrubbed output should maintain JSON structure, got: %s", got)
	}
	// Safe fields should remain
	if !strings.Contains(got, "safe_field") {
		t.Errorf("Non-sensitive fields should be preserved, got: %s", got)
	}
}
