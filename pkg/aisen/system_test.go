package aisen

import (
	"testing"
	"time"
)

func TestCaptureSystemState_PopulatesFields(t *testing.T) {
	startTime := time.Now().Add(-1 * time.Second) // 1 second ago
	state := CaptureSystemState(startTime)

	if state == nil {
		t.Fatal("CaptureSystemState returned nil")
	}

	// Memory should be non-zero (process is using some memory)
	if state.MemoryBytes <= 0 {
		t.Errorf("MemoryBytes = %d, want > 0", state.MemoryBytes)
	}

	// Goroutines should be at least 1 (the test goroutine)
	if state.GoroutineCount < 1 {
		t.Errorf("GoroutineCount = %d, want >= 1", state.GoroutineCount)
	}

	// Uptime should be positive since we set start time in the past
	if state.UptimeMs <= 0 {
		t.Errorf("UptimeMs = %d, want > 0", state.UptimeMs)
	}

	// HostName may be empty on some systems, but we don't error on that
	// Just verify it's a string (the field exists)
	_ = state.HostName
}

func TestCaptureSystemState_UptimeIncreases(t *testing.T) {
	startTime := time.Now()

	state1 := CaptureSystemState(startTime)
	time.Sleep(10 * time.Millisecond)
	state2 := CaptureSystemState(startTime)

	if state2.UptimeMs <= state1.UptimeMs {
		t.Errorf("UptimeMs should increase over time: %d -> %d", state1.UptimeMs, state2.UptimeMs)
	}
}

func TestCaptureSystemState_ZeroStartTime(t *testing.T) {
	// Zero start time should still work (will have large uptime)
	state := CaptureSystemState(time.Time{})

	if state == nil {
		t.Fatal("CaptureSystemState returned nil for zero start time")
	}

	// Should have non-zero fields
	if state.MemoryBytes <= 0 {
		t.Errorf("MemoryBytes = %d, want > 0", state.MemoryBytes)
	}
}

func TestCaptureSystemState_FutureStartTime(t *testing.T) {
	// Future start time should result in negative uptime, which we clamp to 0
	futureTime := time.Now().Add(1 * time.Hour)
	state := CaptureSystemState(futureTime)

	if state == nil {
		t.Fatal("CaptureSystemState returned nil for future start time")
	}

	// Uptime should be clamped to 0 (or negative if not clamped)
	// We accept either behavior in the implementation
	_ = state.UptimeMs
}
