// system.go captures system state at error time.

package aisen

import (
	"os"
	"runtime"
	"time"
)

// CaptureSystemState captures system metrics at the current moment.
// The startTime parameter is used to calculate process uptime.
func CaptureSystemState(startTime time.Time) *SystemState {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	hostname, _ := os.Hostname() // Ignore error, empty hostname is acceptable

	uptimeMs := time.Since(startTime).Milliseconds()
	if uptimeMs < 0 {
		uptimeMs = 0 // Clamp to 0 if start time is in the future
	}

	return &SystemState{
		MemoryBytes:    int64(memStats.Alloc),
		GoroutineCount: runtime.NumGoroutine(),
		UptimeMs:       uptimeMs,
		HostName:       hostname,
	}
}
