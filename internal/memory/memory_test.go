package memory

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/observation"
)

func TestMemoryRecordRoundTrip(t *testing.T) {
	now := time.Date(2026, 3, 28, 8, 30, 0, 0, time.UTC)
	record := MemoryRecord{
		ID:      "memory-001",
		Kind:    MemoryKindSemantic,
		Summary: "User prefers six-month emergency fund coverage.",
		Facts: []MemoryFact{
			{Key: "target_emergency_fund_months", Value: "6", EvidenceID: observation.EvidenceID("evidence-001")},
		},
		Source: MemorySource{
			EvidenceIDs: []observation.EvidenceID{observation.EvidenceID("evidence-001")},
			TaskID:      "task-001",
			WorkflowID:  "workflow-001",
			Actor:       "memory-steward",
		},
		Confidence: MemoryConfidence{Score: 0.88, Rationale: "repeatedly observed in review sessions"},
		CreatedAt:  now,
		UpdatedAt:  now,
		Supersedes: []SupersedesRef{{MemoryID: "memory-old", Reason: "more recent confirmed preference"}},
		Conflicts:  []ConflictRef{{MemoryID: "memory-conflict", Reason: "older contradictory note"}},
	}

	data, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("marshal memory record: %v", err)
	}
	var decoded MemoryRecord
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal memory record: %v", err)
	}
	if err := decoded.Validate(); err != nil {
		t.Fatalf("decoded memory record should validate: %v", err)
	}
}

func TestMemoryRecordInvariantRejectsSelfConflict(t *testing.T) {
	now := time.Now().UTC()
	record := MemoryRecord{
		ID:      "memory-002",
		Kind:    MemoryKindEpisodic,
		Summary: "A failed debt payoff simulation was recorded.",
		Facts:   []MemoryFact{{Key: "simulation", Value: "failed"}},
		Source:  MemorySource{TaskID: "task-1"},
		Confidence: MemoryConfidence{
			Score:     0.7,
			Rationale: "observed directly",
		},
		CreatedAt: now,
		UpdatedAt: now,
		Conflicts: []ConflictRef{{MemoryID: "memory-002", Reason: "self conflict"}},
	}
	if err := record.Validate(); err == nil {
		t.Fatalf("expected self-conflict invariant to fail")
	}
}

func TestDefaultMemoryWriterAndHybridRetriever(t *testing.T) {
	store := NewInMemoryMemoryStore()
	auditLog := &MemoryAccessAuditLog{}
	writer := DefaultMemoryWriter{
		Store:         store,
		AuditLog:      auditLog,
		MinConfidence: 0.75,
	}

	now := time.Date(2026, 3, 28, 12, 0, 0, 0, time.UTC)
	records := []MemoryRecord{
		{
			ID:      "memory-behavior-1",
			Kind:    MemoryKindSemantic,
			Summary: "User shows repeated late-night discretionary spending.",
			Facts: []MemoryFact{
				{Key: "late_night_spending_frequency", Value: "0.4", EvidenceID: observation.EvidenceID("evidence-late-night")},
			},
			Source: MemorySource{
				EvidenceIDs: []observation.EvidenceID{observation.EvidenceID("evidence-late-night")},
				TaskID:      "task-monthly-review",
				WorkflowID:  "workflow-1",
				Actor:       "memory-steward",
			},
			Confidence: MemoryConfidence{Score: 0.9, Rationale: "computed from March transactions"},
			CreatedAt:  now,
			UpdatedAt:  now,
		},
		{
			ID:      "memory-subscriptions-1",
			Kind:    MemoryKindSemantic,
			Summary: "User has duplicate recurring subscriptions across streaming services.",
			Facts: []MemoryFact{
				{Key: "duplicate_subscription_count", Value: "2", EvidenceID: observation.EvidenceID("evidence-subscription")},
			},
			Source: MemorySource{
				EvidenceIDs: []observation.EvidenceID{observation.EvidenceID("evidence-subscription")},
				TaskID:      "task-monthly-review",
				WorkflowID:  "workflow-1",
				Actor:       "memory-steward",
			},
			Confidence: MemoryConfidence{Score: 0.88, Rationale: "merchant-level subscription detection"},
			CreatedAt:  now.Add(time.Minute),
			UpdatedAt:  now.Add(time.Minute),
		},
	}

	for _, record := range records {
		if err := writer.Write(t.Context(), record); err != nil {
			t.Fatalf("write memory %s: %v", record.ID, err)
		}
	}

	lowConfidence := MemoryRecord{
		ID:      "memory-low-confidence",
		Kind:    MemoryKindSemantic,
		Summary: "Weakly inferred preference.",
		Facts:   []MemoryFact{{Key: "weak_signal", Value: "true"}},
		Source: MemorySource{
			EvidenceIDs: []observation.EvidenceID{observation.EvidenceID("evidence-weak")},
			Actor:       "memory-steward",
		},
		Confidence: MemoryConfidence{Score: 0.4, Rationale: "weak"},
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := writer.Write(t.Context(), lowConfidence); err == nil {
		t.Fatalf("expected low-confidence semantic memory to be rejected")
	}

	conflict := MemoryRecord{
		ID:      "memory-subscriptions-2",
		Kind:    MemoryKindSemantic,
		Summary: "User has duplicate recurring subscriptions across streaming services.",
		Facts: []MemoryFact{
			{Key: "duplicate_subscription_count", Value: "3", EvidenceID: observation.EvidenceID("evidence-subscription-2")},
		},
		Source: MemorySource{
			EvidenceIDs: []observation.EvidenceID{observation.EvidenceID("evidence-subscription-2")},
			TaskID:      "task-monthly-review",
			WorkflowID:  "workflow-2",
			Actor:       "memory-steward",
		},
		Confidence: MemoryConfidence{Score: 0.91, Rationale: "newer statement"},
		CreatedAt:  now.Add(2 * time.Minute),
		UpdatedAt:  now.Add(2 * time.Minute),
	}
	if err := writer.Write(t.Context(), conflict); err != nil {
		t.Fatalf("write conflict memory: %v", err)
	}
	stored, ok, err := store.Get(t.Context(), "memory-subscriptions-2")
	if err != nil || !ok {
		t.Fatalf("expected stored conflicting memory, err=%v ok=%v", err, ok)
	}
	if len(stored.Conflicts) == 0 {
		t.Fatalf("expected conflicting memory relation to be preserved")
	}

	retriever := HybridMemoryRetriever{
		Lexical: LexicalRetriever{
			Store:    store,
			AuditLog: auditLog,
		},
		Semantic: SemanticRetriever{
			Store: store,
			Backend: FakeSemanticSearchBackend{
				Provider: KeywordEmbeddingProvider{Dimensions: 12},
				Index:    NewInMemoryVectorIndex(),
				Scorer:   DefaultRetrievalScorer{},
			},
			AuditLog: auditLog,
		},
		Fusion:          ReciprocalRankFusion{},
		Reranker:        BaselineReranker{},
		RejectionPolicy: ThresholdRejectionPolicy{MinScore: 0.01},
	}
	results, err := retriever.Retrieve(t.Context(), RetrievalQuery{
		Text:         "late night spending and subscriptions",
		LexicalTerms: []string{"late", "night", "subscriptions"},
		SemanticHint: "behavioral spending patterns",
		TopK:         2,
	})
	if err != nil {
		t.Fatalf("hybrid retrieve: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("expected hybrid retrieval results")
	}
	if results[0].MemoryID != "memory-subscriptions-2" && results[0].MemoryID != "memory-behavior-1" {
		t.Fatalf("unexpected top result: %+v", results[0])
	}
	if len(auditLog.Entries()) < 3 {
		t.Fatalf("expected audit log entries for writes and retrievals, got %d", len(auditLog.Entries()))
	}
}
