package stderr

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/strongdm/ai-cxdb-observe/pkg/aisen"
)

func TestStderrSink_ImplementsSinkInterface(t *testing.T) {
	var _ aisen.Sink = NewStderrSink()
}

func captureStderr(fn func()) string {
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	fn()

	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	os.Stderr = old
	return buf.String()
}

func TestStderrSink_Write_FormatsOutput(t *testing.T) {
	sink := NewStderrSink()

	event := aisen.ErrorEvent{
		EventID:     "evt-123",
		Timestamp:   time.Date(2025, 1, 26, 15, 4, 5, 0, time.UTC),
		Fingerprint: "abc123def456",
		Severity:    aisen.SeverityError,
		ErrorType:   "panic",
		Message:     "nil pointer dereference",
		Operation:   "tool",
		AgentName:   "researcher",
		ToolName:    "WebSearch",
	}

	output := captureStderr(func() {
		sink.Write(context.Background(), event)
	})

	// Check for expected components in output
	if !strings.Contains(output, "[AISEN]") {
		t.Errorf("Output should contain [AISEN] prefix")
	}
	if !strings.Contains(output, "ERROR") {
		t.Errorf("Output should contain severity ERROR")
	}
	if !strings.Contains(output, "panic") {
		t.Errorf("Output should contain error type 'panic'")
	}
	if !strings.Contains(output, "tool") {
		t.Errorf("Output should contain operation 'tool'")
	}
	if !strings.Contains(output, "WebSearch") {
		t.Errorf("Output should contain tool name 'WebSearch'")
	}
	if !strings.Contains(output, "researcher") {
		t.Errorf("Output should contain agent name 'researcher'")
	}
	if !strings.Contains(output, "nil pointer dereference") {
		t.Errorf("Output should contain message")
	}
	if !strings.Contains(output, "abc123def456") {
		t.Errorf("Output should contain fingerprint")
	}
}

func TestStderrSink_Write_IncludesContext(t *testing.T) {
	sink := NewStderrSink()

	contextID := uint64(12345)
	turnDepth := 7
	event := aisen.ErrorEvent{
		Severity:  aisen.SeverityWarning,
		ErrorType: "test",
		Message:   "test message",
		ContextID: &contextID,
		TurnDepth: &turnDepth,
	}

	output := captureStderr(func() {
		sink.Write(context.Background(), event)
	})

	if !strings.Contains(output, "12345") {
		t.Errorf("Output should contain context ID")
	}
	if !strings.Contains(output, "turn 7") || !strings.Contains(output, "7") {
		t.Errorf("Output should contain turn depth")
	}
}

func TestStderrSink_WithVerbose_IncludesStackTrace(t *testing.T) {
	sink := NewStderrSink(WithVerbose())

	event := aisen.ErrorEvent{
		Severity:   aisen.SeverityCrash,
		ErrorType:  "panic",
		Message:    "test panic",
		StackTrace: "goroutine 1 [running]:\nmain.main()\n\t/app/main.go:10",
	}

	output := captureStderr(func() {
		sink.Write(context.Background(), event)
	})

	if !strings.Contains(output, "goroutine 1") {
		t.Errorf("Verbose output should include stack trace")
	}
	if !strings.Contains(output, "main.main()") {
		t.Errorf("Verbose output should include function names from stack trace")
	}
}

func TestStderrSink_NonVerbose_ExcludesStackTrace(t *testing.T) {
	sink := NewStderrSink() // Not verbose

	event := aisen.ErrorEvent{
		Severity:   aisen.SeverityCrash,
		ErrorType:  "panic",
		Message:    "test panic",
		StackTrace: "goroutine 1 [running]:\nmain.main()\n\t/app/main.go:10",
	}

	output := captureStderr(func() {
		sink.Write(context.Background(), event)
	})

	if strings.Contains(output, "goroutine 1") {
		t.Errorf("Non-verbose output should not include full stack trace")
	}
}

func TestStderrSink_Flush_ReturnsNil(t *testing.T) {
	sink := NewStderrSink()
	err := sink.Flush(context.Background())
	if err != nil {
		t.Errorf("Flush returned error: %v", err)
	}
}

func TestStderrSink_Close_ReturnsNil(t *testing.T) {
	sink := NewStderrSink()
	err := sink.Close()
	if err != nil {
		t.Errorf("Close returned error: %v", err)
	}
}

func TestStderrSink_SeverityFormatting(t *testing.T) {
	tests := []struct {
		severity aisen.Severity
		want     string
	}{
		{aisen.SeverityWarning, "WARNING"},
		{aisen.SeverityError, "ERROR"},
		{aisen.SeverityCrash, "CRASH"},
	}

	for _, tt := range tests {
		t.Run(string(tt.severity), func(t *testing.T) {
			sink := NewStderrSink()
			event := aisen.ErrorEvent{
				Severity:  tt.severity,
				ErrorType: "test",
			}

			output := captureStderr(func() {
				sink.Write(context.Background(), event)
			})

			if !strings.Contains(output, tt.want) {
				t.Errorf("Output should contain %q for severity %q", tt.want, tt.severity)
			}
		})
	}
}
