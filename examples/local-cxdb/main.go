// Example: Local CXDB Testing
//
// This example writes error events to a local cxdb instance for manual testing.
// Run with: go run ./examples/local-cxdb/

package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/strongdm/ai-cxdb-observe/pkg/aisen"
	"github.com/strongdm/ai-cxdb-observe/pkg/aisen/sinks/cxdb"
	cxdbclient "github.com/strongdm/ai-cxdb/clients/go"
)

func main() {
	ctx := context.Background()

	// Connect to local cxdb (binary protocol on port 9009)
	fmt.Println("Connecting to local cxdb at localhost:9009...")
	client, err := cxdbclient.Dial("localhost:9009", cxdbclient.WithClientTag("aisen-local-test"))
	if err != nil {
		log.Fatalf("Failed to connect to cxdb: %v", err)
	}
	defer client.Close()
	fmt.Printf("Connected! Session ID: %d\n", client.SessionID())

	// Create a sink that writes to cxdb
	sink := cxdb.NewCXDBSink(
		client,
		cxdb.WithOrphanLabels([]string{"error", "local-test"}),
		cxdb.WithClientTag("aisen-local-test"),
	)

	// Create a collector with default scrubbing
	collector := aisen.NewCollector(
		aisen.WithSink(sink),
		aisen.WithDefaultScrubbing(),
	)

	fmt.Println("\n=== Recording Error Events to CXDB ===\n")

	// Example 1: Simple error
	fmt.Println("1. Recording simple error...")
	err = collector.Record(ctx, aisen.ErrorEvent{
		Severity:  aisen.SeverityError,
		ErrorType: "connection_error",
		Message:   "failed to connect to external API: timeout after 30s",
		Operation: "http_client",
		AgentName: "data-fetcher",
		Metadata: map[string]string{
			"endpoint": "https://api.example.com/data",
			"attempt":  "3",
		},
	})
	if err != nil {
		log.Printf("Failed to record error 1: %v", err)
	} else {
		fmt.Println("   Recorded!")
	}

	// Example 2: Error with sensitive data (will be scrubbed)
	fmt.Println("2. Recording error with sensitive data (will be scrubbed)...")
	err = collector.Record(ctx, aisen.ErrorEvent{
		Severity:  aisen.SeverityError,
		ErrorType: "auth_failure",
		Message:   "authentication failed for api_key=sk-secret123 user=admin@company.com",
		Operation: "auth",
		AgentName: "auth-agent",
		Metadata: map[string]string{
			"auth_token": "bearer-xyz-secret",
			"user_id":    "12345",
		},
	})
	if err != nil {
		log.Printf("Failed to record error 2: %v", err)
	} else {
		fmt.Println("   Recorded!")
	}

	// Example 3: Tool execution error
	fmt.Println("3. Recording tool execution error...")
	err = collector.Record(ctx, aisen.ErrorEvent{
		Severity:  aisen.SeverityWarning,
		ErrorType: "tool_error",
		Message:   "WebSearch tool returned empty results",
		Operation: "tool",
		AgentName: "research-agent",
		ToolName:  "WebSearch",
		ToolArgs:  `{"query": "latest news"}`,
		Metadata: map[string]string{
			"retry_count": "2",
		},
	})
	if err != nil {
		log.Printf("Failed to record error 3: %v", err)
	} else {
		fmt.Println("   Recorded!")
	}

	// Example 4: Crash/panic simulation
	fmt.Println("4. Recording crash event...")
	startTime := time.Now()
	systemState := aisen.CaptureSystemState(startTime)
	err = collector.Record(ctx, aisen.ErrorEvent{
		Severity:    aisen.SeverityCrash,
		ErrorType:   "panic",
		Message:     "runtime error: index out of range [5] with length 3",
		StackTrace:  "goroutine 1 [running]:\nmain.processItems()\n\t/app/main.go:42\nmain.main()\n\t/app/main.go:15",
		Operation:   "llm",
		AgentName:   "processor-agent",
		SystemState: systemState,
	})
	if err != nil {
		log.Printf("Failed to record error 4: %v", err)
	} else {
		fmt.Println("   Recorded!")
	}

	// Flush to ensure all events are written
	if err := collector.Flush(ctx); err != nil {
		log.Printf("Failed to flush: %v", err)
	}

	fmt.Println("\n=== Done! ===")
	fmt.Println("View errors in the cxdb UI at: http://localhost:8080")
	fmt.Println("Look for contexts with labels: error, local-test")

	// Also demonstrate the Recover helper
	fmt.Println("\n=== Testing Panic Recovery ===")
	func() {
		defer aisen.Recover(ctx, collector)
		defer func() {
			if r := recover(); r != nil {
				fmt.Println("Panic was captured and recorded to cxdb!")
			}
		}()

		// This will panic
		causePanic()
	}()

	// Final flush
	collector.Flush(ctx)
	collector.Close()

	fmt.Println("\nAll done! Check http://localhost:8080 for the error events.")
}

func causePanic() {
	var slice []int
	_ = slice[10] // This will panic
}
