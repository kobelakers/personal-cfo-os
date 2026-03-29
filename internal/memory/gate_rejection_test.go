package memory_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/governance"
	"github.com/kobelakers/personal-cfo-os/internal/memory"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

func TestWorkflowMemoryServiceRejectsMissingEvidenceProvenance(t *testing.T) {
	stores, service := newGateTestService(t, governance.MemoryWritePolicy{
		MinConfidence:   0.7,
		RequireEvidence: true,
		AllowKinds: []memory.MemoryKind{
			memory.MemoryKindEpisodic,
			memory.MemoryKindSemantic,
			memory.MemoryKindProcedural,
		},
	})

	_, err := service.SyncMonthlyReview(
		t.Context(),
		taskspec.TaskSpec{ID: "task-monthly-review", Goal: "月度复盘", Scope: taskspec.TaskScope{Areas: []string{"cashflow"}}},
		"workflow-memory-gate-missing-evidence",
		state.FinancialWorldState{
			UserID: "user-1",
			BehaviorState: state.BehaviorState{
				DuplicateSubscriptionCount: 2,
			},
		},
		nil,
	)
	if err == nil || !memory.IsPolicyDenied(err) {
		t.Fatalf("expected memory write gate rejection for missing evidence provenance, got %v", err)
	}
	recent, err := stores.Query.ListRecent(t.Context(), 10)
	if err != nil {
		t.Fatalf("list recent after rejection: %v", err)
	}
	if len(recent) != 0 {
		t.Fatalf("expected no durable memory after gate rejection, got %+v", recent)
	}
}

func TestWorkflowMemoryServiceRejectsLowConfidenceBeforeDurableWrite(t *testing.T) {
	stores, service := newGateTestService(t, governance.MemoryWritePolicy{
		MinConfidence:   0.8,
		RequireEvidence: false,
		AllowKinds: []memory.MemoryKind{
			memory.MemoryKindEpisodic,
			memory.MemoryKindSemantic,
			memory.MemoryKindProcedural,
		},
	})

	_, err := service.SyncMonthlyReview(
		t.Context(),
		taskspec.TaskSpec{ID: "task-monthly-review", Goal: "月度复盘", Scope: taskspec.TaskScope{Areas: []string{"cashflow"}}},
		"workflow-memory-gate-low-confidence",
		state.FinancialWorldState{
			UserID: "user-1",
			BehaviorState: state.BehaviorState{
				LateNightSpendingFrequency: 0.3,
			},
		},
		[]observation.EvidenceRecord{
			{
				ID:      observation.EvidenceID("evidence-late-night"),
				Type:    observation.EvidenceTypeLateNightSpendingSignal,
				Summary: "late night spending signal",
				Source:  observation.EvidenceSource{Kind: "fixture", Adapter: "test", Reference: "evidence-late-night", Provenance: "fixture"},
				TimeRange: observation.EvidenceTimeRange{
					ObservedAt: time.Date(2026, 3, 31, 8, 0, 0, 0, time.UTC),
				},
				Confidence:    observation.EvidenceConfidence{Score: 0.9, Reason: "test"},
				Normalization: observation.EvidenceNormalizationResult{Status: observation.EvidenceNormalizationNormalized},
			},
		},
	)
	if err == nil || !memory.IsPolicyDenied(err) {
		t.Fatalf("expected memory write gate rejection for low confidence record, got %v", err)
	}
	recent, err := stores.Query.ListRecent(t.Context(), 10)
	if err != nil {
		t.Fatalf("list recent after rejection: %v", err)
	}
	if len(recent) != 0 {
		t.Fatalf("expected no durable memory after low-confidence gate rejection, got %+v", recent)
	}
}

func newGateTestService(t *testing.T, policy governance.MemoryWritePolicy) (*memory.SQLiteMemoryStores, memory.WorkflowMemoryService) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "memory.db")
	stores, err := memory.NewSQLiteMemoryStores(memory.SQLiteStoreConfig{DSN: dbPath})
	if err != nil {
		t.Fatalf("open sqlite memory stores: %v", err)
	}
	t.Cleanup(func() { _ = stores.DB.Close() })
	service := memory.WorkflowMemoryService{
		Writer: memory.DefaultMemoryWriter{
			Store:                      stores.Store,
			MinConfidence:              0.7,
			LowConfidenceEpisodicFloor: 0.55,
			Now: func() time.Time {
				return time.Date(2026, 3, 31, 9, 0, 0, 0, time.UTC)
			},
		},
		Gate: governance.MemoryWriteGateService{
			PolicyEngine:  governance.StaticPolicyEngine{},
			Policy:        policy,
			CorrelationID: "memory-gate-test",
		},
		Now: func() time.Time {
			return time.Date(2026, 3, 31, 9, 0, 0, 0, time.UTC)
		},
	}
	return stores, service
}
