// Example: Standalone Usage
//
// This example demonstrates how to use aisen without ai-agents-sdk.
// It shows manual error recording and panic recovery.

package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/strongdm/ai-cxdb-observe/pkg/aisen"
	"github.com/strongdm/ai-cxdb-observe/pkg/aisen/sinks/stderr"
)

func main() {
	ctx := context.Background()

	// Step 1: Create a sink for error events.
	// For development, stderr is useful for visibility.
	// In production, use cxdb.NewSink() or multi.NewSink() for multiple sinks.
	sink := stderr.NewStderrSink(stderr.WithVerbose())

	// Step 2: Create a collector with default scrubbing.
	collector := aisen.NewCollector(
		aisen.WithSink(sink),
		aisen.WithDefaultScrubbing(),
	)

	fmt.Println("=== Example 1: Manual Error Recording ===")
	manualErrorRecording(ctx, collector)

	fmt.Println("\n=== Example 2: Panic Recovery in Goroutine ===")
	panicRecovery(ctx, collector)

	fmt.Println("\n=== Example 3: Error with Context ID ===")
	errorWithContextID(ctx, collector)

	// Clean up
	if err := collector.Flush(ctx); err != nil {
		fmt.Printf("Failed to flush: %v\n", err)
	}
	if err := collector.Close(); err != nil {
		fmt.Printf("Failed to close: %v\n", err)
	}

	fmt.Println("\nStandalone example completed.")
}

// manualErrorRecording demonstrates manually recording an error event.
func manualErrorRecording(ctx context.Context, collector aisen.Collector) {
	// Simulate an error in your application
	err := errors.New("failed to connect to external service")

	// Create and record an error event manually
	event := aisen.ErrorEvent{
		Severity:  aisen.SeverityError,
		ErrorType: "connection",
		Message:   err.Error(),
		Operation: "http_client",
		AgentName: "my-service",
	}

	if recordErr := collector.Record(ctx, event); recordErr != nil {
		fmt.Printf("Failed to record error: %v\n", recordErr)
	} else {
		fmt.Println("Error event recorded successfully")
	}
}

// panicRecovery demonstrates using Recover helper in goroutines.
func panicRecovery(ctx context.Context, collector aisen.Collector) {
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		// Use Recover to automatically capture panics
		defer aisen.Recover(ctx, collector)

		// Simulate a panic
		fmt.Println("Goroutine starting...")
		time.Sleep(10 * time.Millisecond) // Simulate some work
		panic("something unexpected happened!")
	}()

	wg.Wait()
	fmt.Println("Goroutine completed (panic was captured)")
}

// errorWithContextID demonstrates recording an error with a context ID.
func errorWithContextID(ctx context.Context, collector aisen.Collector) {
	// Assume we have a conversation/context ID from our application
	contextID := uint64(12345)
	ctx = aisen.WithContextID(ctx, contextID)

	// Create an error event with context
	event := aisen.ErrorEvent{
		Severity:  aisen.SeverityWarning,
		ErrorType: "validation",
		Message:   "input validation failed: missing required field 'email'",
		Operation: "validate_input",
		ContextID: &contextID,
	}

	if err := collector.Record(ctx, event); err != nil {
		fmt.Printf("Failed to record error: %v\n", err)
	} else {
		fmt.Println("Warning event recorded with context ID")
	}
}
