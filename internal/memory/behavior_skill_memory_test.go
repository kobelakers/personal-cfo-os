package memory

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/skills"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

func TestWorkflowMemoryServiceWritesBehaviorSkillOutcomeWithOptionalFieldsOmitted(t *testing.T) {
	now := time.Date(2026, 3, 30, 8, 0, 0, 0, time.UTC)
	dbPath := filepath.Join(t.TempDir(), "memory.db")
	stores, err := NewSQLiteMemoryStores(SQLiteStoreConfig{DSN: dbPath})
	if err != nil {
		t.Fatalf("open sqlite memory stores: %v", err)
	}
	t.Cleanup(func() { _ = stores.DB.Close() })

	service := WorkflowMemoryService{
		Writer: DefaultMemoryWriter{
			Store:                      stores.Store,
			Relations:                  stores.Relations,
			AuditStore:                 stores.Audit,
			WriteEventStore:            stores.WriteEvents,
			EmbeddingStore:             stores.Embeddings,
			MinConfidence:              0.7,
			LowConfidenceEpisodicFloor: 0.55,
			Now:                        func() time.Time { return now },
		},
		Now: func() time.Time { return now },
	}

	ids, err := service.WriteBehaviorSkillOutcome(t.Context(), taskspec.TaskSpec{
		ID:             "task-behavior-1",
		Goal:           "行为干预",
		UserIntentType: taskspec.UserIntentBehaviorIntervention,
	}, SkillOutcomeMemory{
		WorkflowID:        "workflow-behavior-1",
		TaskID:            "task-behavior-1",
		TraceID:           "trace-behavior-1",
		SkillFamily:       string(skills.SkillFamilyDiscretionaryGuardrail),
		SkillVersion:      "v1",
		RecipeID:          "soft_nudge.v1",
		FinalRuntimeState: "completed",
	})
	if err != nil {
		t.Fatalf("write behavior skill outcome memory: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("expected one generated memory id, got %+v", ids)
	}

	record, ok, err := stores.Store.Get(t.Context(), ids[0])
	if err != nil {
		t.Fatalf("load behavior skill outcome memory: %v", err)
	}
	if !ok {
		t.Fatalf("expected behavior skill outcome memory to persist")
	}
	if record.Kind != MemoryKindProcedural {
		t.Fatalf("expected procedural memory kind, got %q", record.Kind)
	}
	for _, fact := range record.Facts {
		if fact.Value == "" {
			t.Fatalf("expected optional facts with empty values to be omitted, got %+v", record.Facts)
		}
	}
}

func TestSkillSelectionMemoryRecordsExposeProceduralFacts(t *testing.T) {
	records := []MemoryRecord{
		{
			ID:      "memory-procedural-skill-outcome",
			Kind:    MemoryKindProcedural,
			Summary: "prior guardrail outcome",
			Facts: []MemoryFact{
				{Key: "skill_family", Value: string(skills.SkillFamilyDiscretionaryGuardrail)},
				{Key: "skill_version", Value: "v1"},
				{Key: "recipe_id", Value: "budget_guardrail.v1"},
				{Key: "final_runtime_state", Value: "completed"},
			},
			Source:     MemorySource{TaskID: "task-behavior-1"},
			Confidence: MemoryConfidence{Score: 0.95, Rationale: "workflow-emitted procedural memory"},
			CreatedAt:  time.Date(2026, 3, 30, 8, 0, 0, 0, time.UTC),
			UpdatedAt:  time.Date(2026, 3, 30, 8, 0, 0, 0, time.UTC),
		},
	}

	context := SkillSelectionMemoryRecords(records)
	if len(context) != 1 {
		t.Fatalf("expected one procedural memory context record, got %+v", context)
	}
	if context[0].Facts["skill_family"] != string(skills.SkillFamilyDiscretionaryGuardrail) {
		t.Fatalf("expected skill family fact to be preserved, got %+v", context[0].Facts)
	}
	if context[0].Facts["recipe_id"] != "budget_guardrail.v1" {
		t.Fatalf("expected recipe id fact to be preserved, got %+v", context[0].Facts)
	}
}
