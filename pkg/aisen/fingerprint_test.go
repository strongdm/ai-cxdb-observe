package aisen

import (
	"testing"
	"time"
)

func TestFingerprint_Stability(t *testing.T) {
	event := ErrorEvent{
		EventID:   "evt-123",
		Timestamp: time.Now(),
		Severity:  SeverityError,
		ErrorType: "timeout",
		Message:   "connection timed out",
		Operation: "tool",
		AgentName: "researcher",
		ToolName:  "WebSearch",
		StackTrace: `goroutine 1 [running]:
main.doSomething()
	/app/main.go:42 +0x123
main.helper()
	/app/main.go:30 +0x456
main.main()
	/app/main.go:10 +0x789`,
	}

	fp1 := Fingerprint(event)
	fp2 := Fingerprint(event)

	if fp1 != fp2 {
		t.Errorf("Same event produced different fingerprints: %q vs %q", fp1, fp2)
	}

	// Should be 32 hex characters (16 bytes)
	if len(fp1) != 32 {
		t.Errorf("Fingerprint length = %d, want 32", len(fp1))
	}
}

func TestFingerprint_DifferentLineNumbers_SameFingerprint(t *testing.T) {
	event1 := ErrorEvent{
		ErrorType: "panic",
		Operation: "tool",
		AgentName: "agent1",
		ToolName:  "ToolA",
		StackTrace: `goroutine 1 [running]:
main.doSomething()
	/app/main.go:42 +0x123
main.main()
	/app/main.go:10 +0x456`,
	}

	event2 := ErrorEvent{
		ErrorType: "panic",
		Operation: "tool",
		AgentName: "agent1",
		ToolName:  "ToolA",
		StackTrace: `goroutine 1 [running]:
main.doSomething()
	/app/main.go:99 +0xabc
main.main()
	/app/main.go:55 +0xdef`,
	}

	fp1 := Fingerprint(event1)
	fp2 := Fingerprint(event2)

	if fp1 != fp2 {
		t.Errorf("Events differing only in line numbers should have same fingerprint: %q vs %q", fp1, fp2)
	}
}

func TestFingerprint_DifferentMemoryAddresses_SameFingerprint(t *testing.T) {
	event1 := ErrorEvent{
		ErrorType: "panic",
		Operation: "llm",
		StackTrace: `goroutine 1 [running]:
main.handler(0x1234abcd)
	/app/main.go:42 +0x100`,
	}

	event2 := ErrorEvent{
		ErrorType: "panic",
		Operation: "llm",
		StackTrace: `goroutine 1 [running]:
main.handler(0xdeadbeef)
	/app/main.go:42 +0x200`,
	}

	fp1 := Fingerprint(event1)
	fp2 := Fingerprint(event2)

	if fp1 != fp2 {
		t.Errorf("Events differing only in memory addresses should have same fingerprint: %q vs %q", fp1, fp2)
	}
}

func TestFingerprint_DifferentToolName_DifferentFingerprint(t *testing.T) {
	event1 := ErrorEvent{
		ErrorType: "error",
		Operation: "tool",
		AgentName: "agent1",
		ToolName:  "ToolA",
	}

	event2 := ErrorEvent{
		ErrorType: "error",
		Operation: "tool",
		AgentName: "agent1",
		ToolName:  "ToolB",
	}

	fp1 := Fingerprint(event1)
	fp2 := Fingerprint(event2)

	if fp1 == fp2 {
		t.Error("Events with different tool names should have different fingerprints")
	}
}

func TestFingerprint_DifferentOperation_DifferentFingerprint(t *testing.T) {
	event1 := ErrorEvent{
		ErrorType: "error",
		Operation: "tool",
	}

	event2 := ErrorEvent{
		ErrorType: "error",
		Operation: "llm",
	}

	fp1 := Fingerprint(event1)
	fp2 := Fingerprint(event2)

	if fp1 == fp2 {
		t.Error("Events with different operations should have different fingerprints")
	}
}

func TestFingerprint_DifferentErrorType_DifferentFingerprint(t *testing.T) {
	event1 := ErrorEvent{
		ErrorType: "timeout",
		Operation: "tool",
	}

	event2 := ErrorEvent{
		ErrorType: "panic",
		Operation: "tool",
	}

	fp1 := Fingerprint(event1)
	fp2 := Fingerprint(event2)

	if fp1 == fp2 {
		t.Error("Events with different error types should have different fingerprints")
	}
}

func TestFingerprint_DifferentAgentName_DifferentFingerprint(t *testing.T) {
	event1 := ErrorEvent{
		ErrorType: "error",
		Operation: "tool",
		AgentName: "agent1",
	}

	event2 := ErrorEvent{
		ErrorType: "error",
		Operation: "tool",
		AgentName: "agent2",
	}

	fp1 := Fingerprint(event1)
	fp2 := Fingerprint(event2)

	if fp1 == fp2 {
		t.Error("Events with different agent names should have different fingerprints")
	}
}

func TestFingerprint_EmptyEvent(t *testing.T) {
	event := ErrorEvent{}
	fp := Fingerprint(event)

	// Should still produce a valid fingerprint
	if len(fp) != 32 {
		t.Errorf("Fingerprint length = %d, want 32", len(fp))
	}
}

func TestFingerprint_MessageIgnored(t *testing.T) {
	// Messages often contain variable data, so they should not affect fingerprint
	event1 := ErrorEvent{
		ErrorType: "error",
		Operation: "tool",
		Message:   "Error for user 123",
	}

	event2 := ErrorEvent{
		ErrorType: "error",
		Operation: "tool",
		Message:   "Error for user 456",
	}

	fp1 := Fingerprint(event1)
	fp2 := Fingerprint(event2)

	if fp1 != fp2 {
		t.Errorf("Events differing only in message should have same fingerprint: %q vs %q", fp1, fp2)
	}
}

func TestNormalizeStackTrace(t *testing.T) {
	input := `goroutine 1 [running]:
main.doSomething(0x1234)
	/app/main.go:42 +0x123
pkg.helper()
	/app/pkg/helper.go:20 +0x456
runtime.main()
	/usr/local/go/src/runtime/proc.go:250 +0x789
another.function()
	/app/another.go:100 +0xabc`

	frames := normalizeStackTrace(input)

	// Should return first 3 function names
	if len(frames) != 3 {
		t.Errorf("normalizeStackTrace returned %d frames, want 3", len(frames))
	}

	expected := []string{"main.doSomething", "pkg.helper", "runtime.main"}
	for i, want := range expected {
		if i < len(frames) && frames[i] != want {
			t.Errorf("frame[%d] = %q, want %q", i, frames[i], want)
		}
	}
}
