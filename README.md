# ai-cxdb-observe (aisen)

Lightweight, pluggable error and crash collection for AI agent systems. Captures rich error context from ai-agents-sdk sessions and stores it in cxdb for correlation with conversation history.

## Installation

```bash
go get github.com/strongdm/ai-cxdb-observe
```

## Quick Start

### With AI Agents SDK

```go
import (
    "github.com/strongdm/ai-agents-sdk/pkg/agents"
    llmconfig "github.com/strongdm/ai-llm-sdk/pkg/config"
    "github.com/strongdm/ai-cxdb-observe/pkg/aisen"
    "github.com/strongdm/ai-cxdb-observe/pkg/aisen/adapters/agentssdk"
    "github.com/strongdm/ai-cxdb-observe/pkg/aisen/sinks/stderr"
)

func main() {
    ctx := context.Background()

    // Create sink (stderr for dev, cxdb for prod)
    sink := stderr.NewStderrSink(stderr.WithVerbose())

    // Create collector with scrubbing
    collector := aisen.NewCollector(
        aisen.WithSink(sink),
        aisen.WithDefaultScrubbing(),
    )
    defer collector.Close()

    // Create LLM client and runner
    llmClient, _ := llmconfig.NewClientFromEnv()
    baseRunner := agents.NewRunner(llmClient)

    // Instrument with aisen - one line!
    wrappedRunner := agentssdk.Instrument(baseRunner, collector)

    // Use as normal - errors automatically captured
    agent := agents.NewAgent(agents.AgentConfig{
        Name: "my-agent",
        Instructions: "You are a helpful assistant.",
    })
    result, err := wrappedRunner.Run(ctx, agent, "Hello!", nil, nil)
}
```

### Standalone Usage

```go
import (
    "github.com/strongdm/ai-cxdb-observe/pkg/aisen"
    "github.com/strongdm/ai-cxdb-observe/pkg/aisen/sinks/stderr"
)

func main() {
    sink := stderr.NewStderrSink()
    collector := aisen.NewCollector(aisen.WithSink(sink))
    defer collector.Close()

    // Automatic panic recovery in goroutines
    go func() {
        defer aisen.Recover(ctx, collector)
        // code that might panic
    }()

    // Manual error recording
    event := aisen.ErrorEvent{
        Severity:  aisen.SeverityError,
        ErrorType: "connection",
        Message:   "failed to connect to database",
        Operation: "db_connect",
    }
    collector.Record(ctx, event)
}
```

## Features

When AI agents fail (crashes, timeouts, tool errors), aisen automatically captures:

- **What operation was running** (tool, LLM call, guardrail)
- **Full stack trace** (scrubbed of sensitive data)
- **System state** (memory, goroutines)
- **Conversation context** (links error to the exact turn via ContextID)
- **Fingerprint** for grouping similar errors
- **Operation history** (last 10 operations leading to the error)

## Operation History

When errors occur, aisen captures the last 10 operations (LLM calls and tool executions) that led to the failure. This provides a "breadcrumb trail" for debugging agent behavior and understanding failure patterns.

### What's Captured

For **LLM operations**, aisen records:
- Model name and provider
- Message count and metadata (role, length, flags)
- Request parameters (temperature, max_tokens)
- Tool definitions (names only, not schemas)
- Response metadata (finish reason, token usage)
- Duration

For **tool operations**, aisen records:
- Tool name and call ID
- Input/output sizes
- Duration
- Scrubbed input/output (sensitive data removed)

### What's NOT Captured

For security and privacy:
- **Message text is never stored** - only metadata (length, role, content type)
- **Tool schemas are omitted** - only tool names
- **All data is scrubbed** for API keys, tokens, passwords, and PII

### Memory & Performance

- **Bounded memory**: Maximum 10 operations, ~100KB worst case per error
- **Minimal overhead**: <1ms per operation
- **Automatic cleanup**: History cleared between runs
- **Fail-closed scrubbing**: Invalid data is redacted, not exposed

### Accessing Operation History

**In cxdb portal**: Operation history appears as a top-level `operation_history` array in error details, showing the sequence of operations with full metadata.

**In stderr sink** (verbose mode):
```
Operation History (2 operations):
  1. [llm] 15:04:05 agent=researcher (1.234s)
     Model: gpt-4 (openai)
     Tokens: 100 prompt + 50 completion = 150 total
     Finish: stop
  2. [tool] 15:04:07 agent=researcher (0.567s)
     Tool: WebSearch (id: call-123)
     I/O: 42 bytes in, 1024 bytes out
```

**Programmatically**: Operation history is stored as JSON in `ErrorEvent.Metadata["aisen.operation_history_json"]` and can be parsed for custom analysis.

### Use Cases

- **Debug intermittent failures**: See the exact sequence of LLM calls and tool executions
- **Analyze token usage**: Track cumulative token consumption leading to errors
- **Performance investigation**: Identify slow operations that preceded crashes
- **Cost attribution**: Understand token costs of failed attempts
- **Pattern detection**: Identify common operation sequences that trigger errors

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        Application                              │
│  (ai-agents-sdk, other Go services)                            │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                      Adapters Layer                             │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐             │
│  │ RunWrapper  │  │ Hook        │  │ Panic       │             │
│  │ (errors)    │  │ Adapter     │  │ Recovery    │             │
│  └─────────────┘  └─────────────┘  └─────────────┘             │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                      Core: Collector                            │
│  - ErrorEvent struct (enriched payload)                        │
│  - Fingerprinting (group similar errors)                       │
│  - Sensitive data scrubbing (fail-closed)                      │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                       Sinks Layer                               │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐             │
│  │ cxdb        │  │ stderr      │  │ multi       │             │
│  │ (prod)      │  │ (dev)       │  │ (fan-out)   │             │
│  │             │  │             │  │             │             │
│  │ async       │  │ noop        │  │             │             │
│  │ (buffered)  │  │ (testing)   │  │             │             │
│  └─────────────┘  └─────────────┘  └─────────────┘             │
└─────────────────────────────────────────────────────────────────┘
```

## Sinks

| Sink | Description | Use Case |
|------|-------------|----------|
| `stderr` | Human-readable logs | Development, debugging |
| `cxdb` | CXDB storage via msgpack | Production |
| `multi` | Fan-out to multiple sinks | Log + persist |
| `async` | Buffered async delivery | High-throughput |
| `noop` | Discards events | Testing |

### Using Multiple Sinks

```go
import (
    "github.com/strongdm/ai-cxdb-observe/pkg/aisen/sinks/multi"
    "github.com/strongdm/ai-cxdb-observe/pkg/aisen/sinks/stderr"
    "github.com/strongdm/ai-cxdb-observe/pkg/aisen/sinks/cxdb"
)

// Fan out to both stderr and cxdb
sink := multi.NewSink(
    stderr.NewStderrSink(stderr.WithVerbose()),
    cxdb.NewCXDBSink(cxdbClient),
)
```

### Async Sink for High Throughput

```go
import "github.com/strongdm/ai-cxdb-observe/pkg/aisen/sinks/async"

// Buffered async delivery with bounded queue
asyncSink := async.NewSink(innerSink,
    async.WithQueueSize(1000),
    async.WithDropOldest(), // Drop oldest on overflow
)
defer asyncSink.Close() // Flushes remaining events
```

## Scrubbing

By default, aisen scrubs sensitive data from error messages:

- API keys (OpenAI, Anthropic, etc.)
- Bearer tokens
- Email addresses
- Numeric sequences (phone numbers, SSNs)

Scrubbing is **fail-closed**: if scrubbing fails, the content is fully redacted rather than persisted raw.

```go
// Enable default scrubbing
collector := aisen.NewCollector(
    aisen.WithSink(sink),
    aisen.WithDefaultScrubbing(),
)

// Or customize
collector := aisen.NewCollector(
    aisen.WithSink(sink),
    aisen.WithScrubber(aisen.ScrubberConfig{
        ScrubAPIKeys: true,
        ScrubEmails:  true,
        ScrubNumbers: false, // Allow numbers
    }),
)
```

## Error Types

| Severity | Description |
|----------|-------------|
| `warning` | Non-fatal issues |
| `error` | Recoverable errors |
| `crash` | Unrecoverable (panics) |

| ErrorType | Description |
|-----------|-------------|
| `error` | Generic error |
| `timeout` | context.DeadlineExceeded |
| `canceled` | context.Canceled |
| `guardrail` | Safety/policy violations |
| `panic` | Recovered panics |

## Examples

See the `examples/` directory:

- `examples/agentssdk/` - Integration with ai-agents-sdk
- `examples/standalone/` - Standalone usage without agents SDK

## Documentation

- `docs/DESIGN.md` - Technical design document
- `docs/implementation-plan.md` - Implementation details

## Related Repositories

- [ai-cxdb](https://github.com/strongdm/ai-cxdb) - Storage backend and portal UI
- [ai-agents-sdk](https://github.com/strongdm/ai-agents-sdk) - AI agent framework
