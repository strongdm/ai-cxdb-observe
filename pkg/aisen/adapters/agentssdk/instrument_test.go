package agentssdk

import (
	"log"
	"os"
	"testing"
)

func TestInstrument_ReturnsWrappedRunner(t *testing.T) {
	collector := &testCollector{}

	// Instrument with nil runner (we're just testing the wrapper creation)
	wrapped := Instrument(nil, collector)

	if wrapped == nil {
		t.Fatal("Instrument returned nil")
	}
}

func TestInstrument_PassesOptions(t *testing.T) {
	collector := &testCollector{}
	store := NewEnrichmentStore()
	logger := log.New(os.Stderr, "test: ", 0)

	wrapped := Instrument(nil, collector,
		WithEnrichmentStore(store),
		WithLogger(logger),
	)

	if wrapped == nil {
		t.Fatal("Instrument returned nil")
	}

	// Verify enrichment store was set by using it
	store.Update("test-run", func(e *Enrichment) {
		e.AgentName = "test-agent"
	})

	// The wrapper should use the same store
	enrichment, ok := wrapped.enrichments.Get("test-run")
	if !ok {
		t.Fatal("Enrichment store was not properly set")
	}
	if enrichment.AgentName != "test-agent" {
		t.Errorf("AgentName = %q, want %q", enrichment.AgentName, "test-agent")
	}
}

func TestInstrument_DefaultEnrichmentStore(t *testing.T) {
	collector := &testCollector{}

	wrapped := Instrument(nil, collector)

	// Should have a default enrichment store
	if wrapped.enrichments == nil {
		t.Fatal("Default enrichment store not created")
	}
}

func TestWithLogger_SetsLogger(t *testing.T) {
	collector := &testCollector{}
	logger := log.New(os.Stderr, "custom: ", 0)

	wrapped := Instrument(nil, collector, WithLogger(logger))

	if wrapped.logger != logger {
		t.Error("Logger was not properly set")
	}
}

func TestWithEnrichmentStore_SetsStore(t *testing.T) {
	collector := &testCollector{}
	store := NewEnrichmentStore()

	wrapped := Instrument(nil, collector, WithEnrichmentStore(store))

	if wrapped.enrichments != store {
		t.Error("Enrichment store was not properly set")
	}
}
