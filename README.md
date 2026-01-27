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
