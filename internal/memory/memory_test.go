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
