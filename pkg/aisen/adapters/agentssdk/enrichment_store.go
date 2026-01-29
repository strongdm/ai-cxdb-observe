// enrichment_store.go provides thread-safe storage for per-run enrichment data
// that correlates hooks with RunWrapper errors.

package agentssdk

import "sync"

// Enrichment contains per-run context captured from hooks.
// This data is merged into ErrorEvents when errors occur.
type Enrichment struct {
	// AgentName is the name of the agent that was running.
	AgentName string

	// Model is the LLM model being used.
	Model string

	// ToolName is the name of the tool being called.
	ToolName string

	// ToolCallID is the unique ID of the tool call.
	ToolCallID string

	// Operation indicates what type of operation was in progress (tool, llm, guardrail, handoff).
	Operation string

	// OperationID is an identifier for the specific operation.
	OperationID string

	// operationHistory stores the last N operations (LLM calls, tool calls).
	// Private field - not exposed in public API to maintain backwards compatibility.
	operationHistory *operationHistoryBuffer
}

// EnrichmentStore provides thread-safe storage for per-run enrichment data.
// Implementations must be safe for concurrent use.
type EnrichmentStore interface {
	// Update applies fn to the enrichment for runID, creating it if needed.
	// IMPORTANT: fn is called while holding the lock. fn MUST be fast and
	// MUST NOT call other EnrichmentStore methods (deadlock risk).
	Update(runID string, fn func(e *Enrichment))

	// Get returns a copy of the enrichment for runID.
	// Returns zero value and false if not found.
	Get(runID string) (Enrichment, bool)

	// Delete removes the enrichment for runID.
	Delete(runID string)
}

// inMemoryEnrichmentStore is the default EnrichmentStore implementation.
type inMemoryEnrichmentStore struct {
	mu   sync.RWMutex
	data map[string]*Enrichment
}

// NewEnrichmentStore creates a new in-memory enrichment store.
func NewEnrichmentStore() EnrichmentStore {
	return &inMemoryEnrichmentStore{
		data: make(map[string]*Enrichment),
	}
}

// Update applies fn to the enrichment for runID, creating it if needed.
func (s *inMemoryEnrichmentStore) Update(runID string, fn func(e *Enrichment)) {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, ok := s.data[runID]
	if !ok {
		e = &Enrichment{}
		s.data[runID] = e
	}
	fn(e) // Called under lock - must be fast!
}

// Get returns a copy of the enrichment for runID.
func (s *inMemoryEnrichmentStore) Get(runID string) (Enrichment, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	e, ok := s.data[runID]
	if !ok {
		return Enrichment{}, false
	}
	// Return a copy to prevent external modification
	return *e, true
}

// Delete removes the enrichment for runID.
func (s *inMemoryEnrichmentStore) Delete(runID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, runID)
}

// RecordOperation adds an operation record to the enrichment's history.
// Initializes the buffer on first call. This method is not part of the
// EnrichmentStore interface but is available on the concrete Enrichment type.
func (e *Enrichment) RecordOperation(record OperationRecord) {
	// Initialize buffer on first use
	if e.operationHistory == nil {
		e.operationHistory = &operationHistoryBuffer{
			maxSize: 10, // Default size, can be made configurable later
		}
	}
	e.operationHistory.Add(record)
}

// GetOperationHistory returns all operation records in chronological order.
// Returns an empty slice if no operations have been recorded.
// This method is not part of the EnrichmentStore interface but is available
// on the concrete Enrichment type.
func (e *Enrichment) GetOperationHistory() []OperationRecord {
	if e.operationHistory == nil {
		return []OperationRecord{}
	}
	return e.operationHistory.GetAll()
}
