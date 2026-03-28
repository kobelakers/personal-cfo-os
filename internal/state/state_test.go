package state

import (
	"testing"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/observation"
)

func TestEvidencePatchUpdatesFinancialWorldState(t *testing.T) {
	now := time.Date(2026, 3, 28, 9, 0, 0, 0, time.UTC)
	current := FinancialWorldState{
		UserID: "user-1",
		WorkflowState: WorkflowState{
			Phase:         "plan",
			LastUpdatedAt: now.Add(-time.Hour),
		},
		Version: StateVersion{
			Sequence:   1,
			SnapshotID: "state-v1",
			UpdatedAt:  now.Add(-time.Hour),
		},
	}

	evidence := observation.EvidenceRecord{
		ID:   observation.EvidenceID("evidence-payroll"),
		Type: observation.EvidenceTypeLedgerTransaction,
		Source: observation.EvidenceSource{
			Kind:       "ledger",
			Adapter:    "csv-import",
			Reference:  "payroll-2026-03",
			Provenance: "import-run-2",
		},
		TimeRange: observation.EvidenceTimeRange{ObservedAt: now},
		Confidence: observation.EvidenceConfidence{
			Score:  0.95,
			Reason: "statement line matched",
		},
		Claims: []observation.EvidenceClaim{
			{Subject: "cashflow", Predicate: "monthly_net_income_cents", Object: "month", ValueJSON: "1800000"},
		},
		Normalization: observation.EvidenceNormalizationResult{Status: observation.EvidenceNormalizationNormalized},
		Summary:       "monthly payroll",
		CreatedAt:     now,
	}

	patch := EvidencePatch{
		Evidence: []observation.EvidenceRecord{evidence},
		Mutations: []StateMutation{
			{Path: "cashflow.monthly_net_income_cents", Operation: "replace", ValueJSON: "1800000", EvidenceID: evidence.ID},
			{Path: "workflow.phase", Operation: "replace", ValueJSON: "\"act\"", EvidenceID: evidence.ID},
		},
		Summary:   "apply payroll evidence",
		AppliedAt: now,
	}

	next, diff, err := DefaultStateReducer{}.ApplyEvidencePatch(current, patch)
	if err != nil {
		t.Fatalf("apply evidence patch: %v", err)
	}
	if next.CashflowState.MonthlyNetIncomeCents != 1800000 {
		t.Fatalf("unexpected monthly net income: %d", next.CashflowState.MonthlyNetIncomeCents)
	}
	if next.WorkflowState.Phase != "act" {
		t.Fatalf("unexpected workflow phase: %q", next.WorkflowState.Phase)
	}
	if diff.FromVersion != 1 || diff.ToVersion != 2 {
		t.Fatalf("unexpected state diff: %+v", diff)
	}
	snapshot := next.Snapshot("after payroll", now)
	if snapshot.State.Version.Sequence != next.Version.Sequence {
		t.Fatalf("snapshot should preserve state version")
	}
}
