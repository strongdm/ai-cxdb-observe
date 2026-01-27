package agentssdk

import (
	"sync"
	"testing"
)

func TestEnrichmentStore_Update_CreatesNew(t *testing.T) {
	store := NewEnrichmentStore()

	store.Update("run-1", func(e *Enrichment) {
		e.AgentName = "agent1"
		e.ToolName = "tool1"
	})

	got, ok := store.Get("run-1")
	if !ok {
		t.Fatal("Get returned ok=false for existing key")
	}
	if got.AgentName != "agent1" {
		t.Errorf("AgentName = %q, want %q", got.AgentName, "agent1")
	}
	if got.ToolName != "tool1" {
		t.Errorf("ToolName = %q, want %q", got.ToolName, "tool1")
	}
}

func TestEnrichmentStore_Update_ModifiesExisting(t *testing.T) {
	store := NewEnrichmentStore()

	// First update
	store.Update("run-1", func(e *Enrichment) {
		e.AgentName = "agent1"
	})

	// Second update should modify same entry
	store.Update("run-1", func(e *Enrichment) {
		e.ToolName = "tool1"
	})

	got, ok := store.Get("run-1")
	if !ok {
		t.Fatal("Get returned ok=false")
	}

	// Both values should be present
	if got.AgentName != "agent1" {
		t.Errorf("AgentName = %q, want %q", got.AgentName, "agent1")
	}
	if got.ToolName != "tool1" {
		t.Errorf("ToolName = %q, want %q", got.ToolName, "tool1")
	}
}

func TestEnrichmentStore_Get_ReturnsCopy(t *testing.T) {
	store := NewEnrichmentStore()

	store.Update("run-1", func(e *Enrichment) {
		e.AgentName = "original"
	})

	// Get a copy
	got, _ := store.Get("run-1")

	// Modify the returned copy
	got.AgentName = "modified"

	// Original should be unchanged
	original, _ := store.Get("run-1")
	if original.AgentName != "original" {
		t.Errorf("Modifying returned value affected store: AgentName = %q, want %q", original.AgentName, "original")
	}
}

func TestEnrichmentStore_Get_NotFound(t *testing.T) {
	store := NewEnrichmentStore()

	_, ok := store.Get("nonexistent")
	if ok {
		t.Error("Get should return ok=false for nonexistent key")
	}
}

func TestEnrichmentStore_Delete_RemovesEntry(t *testing.T) {
	store := NewEnrichmentStore()

	store.Update("run-1", func(e *Enrichment) {
		e.AgentName = "agent1"
	})

	// Verify it exists
	_, ok := store.Get("run-1")
	if !ok {
		t.Fatal("Entry should exist before delete")
	}

	// Delete
	store.Delete("run-1")

	// Verify it's gone
	_, ok = store.Get("run-1")
	if ok {
		t.Error("Entry should not exist after delete")
	}
}

func TestEnrichmentStore_Delete_Nonexistent(t *testing.T) {
	store := NewEnrichmentStore()

	// Should not panic
	store.Delete("nonexistent")
}

func TestEnrichmentStore_ConcurrentAccess(t *testing.T) {
	store := NewEnrichmentStore()
	var wg sync.WaitGroup

	// Concurrent updates
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			runID := "run-" + string(rune('0'+i%10))
			store.Update(runID, func(e *Enrichment) {
				e.AgentName = "agent"
			})
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			runID := "run-" + string(rune('0'+i%10))
			store.Get(runID)
		}(i)
	}

	// Concurrent deletes
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			runID := "run-" + string(rune('0'+i%10))
			store.Delete(runID)
		}(i)
	}

	wg.Wait()
	// If we get here without data races, test passes
}

func TestEnrichment_AllFields(t *testing.T) {
	e := Enrichment{
		AgentName:   "agent1",
		Model:       "gpt-4",
		ToolName:    "WebSearch",
		ToolCallID:  "call-123",
		Operation:   "tool",
		OperationID: "op-456",
	}

	if e.AgentName != "agent1" {
		t.Error("AgentName not set correctly")
	}
	if e.Model != "gpt-4" {
		t.Error("Model not set correctly")
	}
	if e.ToolName != "WebSearch" {
		t.Error("ToolName not set correctly")
	}
	if e.ToolCallID != "call-123" {
		t.Error("ToolCallID not set correctly")
	}
	if e.Operation != "tool" {
		t.Error("Operation not set correctly")
	}
	if e.OperationID != "op-456" {
		t.Error("OperationID not set correctly")
	}
}
