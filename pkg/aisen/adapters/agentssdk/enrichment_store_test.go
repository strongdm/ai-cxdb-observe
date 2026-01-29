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

// TestEnrichmentRecordOperation verifies buffer initialization and recording.
func TestEnrichmentRecordOperation(t *testing.T) {
	e := &Enrichment{}

	// Buffer should be nil initially
	if e.operationHistory != nil {
		t.Error("operationHistory should be nil initially")
	}

	// Record first operation - should initialize buffer
	rec1 := OperationRecord{Kind: "llm", AgentName: "agent1"}
	e.RecordOperation(rec1)

	// Buffer should now exist
	if e.operationHistory == nil {
		t.Fatal("operationHistory should be initialized after first record")
	}

	// Verify record was added
	history := e.GetOperationHistory()
	if len(history) != 1 {
		t.Fatalf("history length = %d, want 1", len(history))
	}
	if history[0].Kind != "llm" {
		t.Errorf("history[0].Kind = %q, want %q", history[0].Kind, "llm")
	}
	if history[0].AgentName != "agent1" {
		t.Errorf("history[0].AgentName = %q, want %q", history[0].AgentName, "agent1")
	}

	// Record second operation
	rec2 := OperationRecord{Kind: "tool", AgentName: "agent2"}
	e.RecordOperation(rec2)

	history = e.GetOperationHistory()
	if len(history) != 2 {
		t.Fatalf("history length = %d, want 2", len(history))
	}
	if history[1].AgentName != "agent2" {
		t.Errorf("history[1].AgentName = %q, want %q", history[1].AgentName, "agent2")
	}
}

// TestEnrichmentOperationHistoryIsolation verifies per-run isolation.
func TestEnrichmentOperationHistoryIsolation(t *testing.T) {
	store := NewEnrichmentStore()

	// Record operations for run-1
	store.Update("run-1", func(e *Enrichment) {
		e.RecordOperation(OperationRecord{Kind: "llm", AgentName: "agent1"})
		e.RecordOperation(OperationRecord{Kind: "tool", AgentName: "agent1"})
	})

	// Record operations for run-2
	store.Update("run-2", func(e *Enrichment) {
		e.RecordOperation(OperationRecord{Kind: "llm", AgentName: "agent2"})
	})

	// Verify run-1 history
	e1, ok := store.Get("run-1")
	if !ok {
		t.Fatal("run-1 should exist")
	}
	history1 := e1.GetOperationHistory()
	if len(history1) != 2 {
		t.Errorf("run-1 history length = %d, want 2", len(history1))
	}
	if history1[0].AgentName != "agent1" {
		t.Errorf("run-1 history[0].AgentName = %q, want %q", history1[0].AgentName, "agent1")
	}

	// Verify run-2 history
	e2, ok := store.Get("run-2")
	if !ok {
		t.Fatal("run-2 should exist")
	}
	history2 := e2.GetOperationHistory()
	if len(history2) != 1 {
		t.Errorf("run-2 history length = %d, want 1", len(history2))
	}
	if history2[0].AgentName != "agent2" {
		t.Errorf("run-2 history[0].AgentName = %q, want %q", history2[0].AgentName, "agent2")
	}

	// Delete run-1 should not affect run-2
	store.Delete("run-1")
	e2Again, ok := store.Get("run-2")
	if !ok {
		t.Fatal("run-2 should still exist after run-1 deleted")
	}
	history2Again := e2Again.GetOperationHistory()
	if len(history2Again) != 1 {
		t.Errorf("run-2 history should be unchanged, got length %d", len(history2Again))
	}
}

// TestEnrichmentGetOperationHistoryNilBuffer verifies graceful handling of nil buffer.
func TestEnrichmentGetOperationHistoryNilBuffer(t *testing.T) {
	e := &Enrichment{}

	// GetOperationHistory should return empty slice when buffer is nil
	history := e.GetOperationHistory()
	if history == nil {
		t.Error("GetOperationHistory should return non-nil slice")
	}
	if len(history) != 0 {
		t.Errorf("GetOperationHistory length = %d, want 0", len(history))
	}
}
