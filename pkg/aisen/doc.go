// Package aisen provides lightweight, pluggable error and crash collection
// for AI agent systems.
//
// aisen captures rich error context from ai-agents-sdk sessions and stores it
// in cxdb for correlation with conversation history. This enables engineers to
// debug agent failures with full context about what the agent was doing when
// it failed.
//
// # Core Components
//
// The library is organized around these concepts:
//
//   - ErrorEvent: The canonical error representation with severity, context, and metadata
//   - Collector: Central abstraction that applies scrubbing and fingerprinting before persistence
//   - Sink: Destination for error events (cxdb, stderr, async, multi, noop)
//   - Scrubber: Redacts sensitive data with fail-closed behavior
//
// # Quick Start
//
// For ai-agents-sdk integration:
//
//	collector := aisen.NewCollector(
//	    aisen.WithSink(cxdb.NewCXDBSink(client)),
//	    aisen.WithDefaultScrubbing(),
//	)
//	runner := agentssdk.Instrument(baseRunner, collector)
//	result, err := runner.Run(ctx, agent, input, session, cfg)
//
// For standalone usage:
//
//	collector := aisen.NewCollector(aisen.WithSink(stderr.NewStderrSink()))
//	defer aisen.Recover(ctx, collector)
//
// # Design Principles
//
//   - Adapters never abort agent runs: all collector errors are swallowed and logged
//   - Fail-closed scrubbing: on any error, fields are fully redacted (never persist raw data)
//   - Zero-dependency core: external dependencies only in sink/adapter packages
package aisen
