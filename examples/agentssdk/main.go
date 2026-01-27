// Example: AI Agents SDK Integration
//
// This example demonstrates how to integrate aisen with ai-agents-sdk
// for automatic error and panic capture in agent runs.
//
// Note: This example won't run successfully without proper LLM credentials.
// It's meant to show the integration pattern.

package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/strongdm/ai-agents-sdk/pkg/agents"
	llmconfig "github.com/strongdm/ai-llm-sdk/pkg/config"
	"github.com/strongdm/ai-cxdb-observe/pkg/aisen"
	"github.com/strongdm/ai-cxdb-observe/pkg/aisen/adapters/agentssdk"
	"github.com/strongdm/ai-cxdb-observe/pkg/aisen/sinks/stderr"
)

func main() {
	ctx := context.Background()

	// Step 1: Create a sink for error events.
	// In production, use cxdb.NewSink() to send events to CXDB.
	// For this example, we use stderr for visibility.
	sink := stderr.NewStderrSink(stderr.WithVerbose())

	// Step 2: Create a collector with default scrubbing.
	// The collector processes events through the scrubbing pipeline
	// before sending them to the sink.
	collector := aisen.NewCollector(
		aisen.WithSink(sink),
		aisen.WithDefaultScrubbing(),
	)

	// Step 3: Create your base LLM client and Runner.
	// This uses environment variables for configuration:
	// - OPENAI_API_KEY for OpenAI
	// - ANTHROPIC_API_KEY for Anthropic
	// etc.
	llmClient, err := llmconfig.NewClientFromEnv()
	if err != nil {
		// In a real application, you'd handle this differently.
		// For this example, we'll continue with a nil client to show the pattern.
		log.Printf("Note: Could not create LLM client: %v", err)
		log.Printf("This is expected if no API keys are configured.")
	}

	baseRunner := agents.NewRunner(llmClient)

	// Step 4: Instrument the runner with aisen.
	// This wraps the runner to automatically capture errors and panics.
	logger := log.New(os.Stderr, "aisen: ", log.LstdFlags)
	wrappedRunner := agentssdk.Instrument(
		baseRunner,
		collector,
		agentssdk.WithLogger(logger),
	)

	// Step 5: Create an agent.
	agent := agents.NewAgent(agents.AgentConfig{
		Name:         "example-agent",
		Instructions: "You are a helpful assistant.",
	})

	// Step 6: Run the agent.
	// Any errors returned by Run() will be automatically captured.
	// Panics will also be captured, recorded, and re-raised.
	result, err := wrappedRunner.Run(ctx, agent, "Hello, world!", nil, nil)
	if err != nil {
		// The error has already been recorded to the collector.
		// You can handle it as normal in your application.
		fmt.Printf("Agent run failed (expected without credentials): %v\n", err)
	} else {
		fmt.Printf("Agent response: %+v\n", result)
	}

	// Step 7: Flush and close the collector when shutting down.
	// This ensures all events are written before the application exits.
	if err := collector.Flush(ctx); err != nil {
		log.Printf("Failed to flush collector: %v", err)
	}
	if err := collector.Close(); err != nil {
		log.Printf("Failed to close collector: %v", err)
	}

	fmt.Println("Example completed.")
}
