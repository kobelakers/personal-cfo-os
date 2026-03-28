package state_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/reducers"
	"github.com/kobelakers/personal-cfo-os/internal/state"
)

func TestEvidencePatchUpdatesFinancialWorldState(t *testing.T) {
	now := time.Date(2026, 3, 28, 9, 0, 0, 0, time.UTC)
	current := state.FinancialWorldState{
		UserID: "user-1",
		WorkflowState: state.WorkflowState{
			Phase:         "plan",
			LastUpdatedAt: now.Add(-time.Hour),
		},
		Version: state.StateVersion{
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

	patch := state.EvidencePatch{
		Evidence: []observation.EvidenceRecord{evidence},
		Mutations: []state.StateMutation{
			{Path: "cashflow.monthly_net_income_cents", Operation: "replace", ValueJSON: "1800000", EvidenceID: evidence.ID},
			{Path: "workflow.phase", Operation: "replace", ValueJSON: "\"act\"", EvidenceID: evidence.ID},
		},
		Summary:   "apply payroll evidence",
		AppliedAt: now,
	}

	next, diff, err := state.DefaultStateReducer{}.ApplyEvidencePatch(current, patch)
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

func TestEvidenceDrivenReducersUpdateFinancialWorldState(t *testing.T) {
	transactions, err := observation.LoadLedgerTransactionsCSV(mustReadStateFixture(t, "ledger_transactions_2026-03.csv"))
	if err != nil {
		t.Fatalf("load transactions: %v", err)
	}
	debts, err := observation.LoadDebtRecordsCSV(mustReadStateFixture(t, "debts_2026-03.csv"))
	if err != nil {
		t.Fatalf("load debts: %v", err)
	}
	holdings, err := observation.LoadHoldingRecordsCSV(mustReadStateFixture(t, "holdings_2026-03.csv"))
	if err != nil {
		t.Fatalf("load holdings: %v", err)
	}

	now := time.Date(2026, 3, 28, 11, 0, 0, 0, time.UTC)
	ledger := observation.LedgerObservationAdapter{
		Transactions: transactions,
		Debts:        debts,
		Holdings:     holdings,
		Now:          func() time.Time { return now },
	}
	ledgerEvidence, err := ledger.Observe(t.Context(), observation.ObservationRequest{
		TaskID:     "task-monthly-review",
		SourceKind: "ledger",
		Params: map[string]string{
			"user_id": "user-1",
			"start":   "2026-03-01",
			"end":     "2026-03-31T23:59:59Z",
		},
	})
	if err != nil {
		t.Fatalf("observe ledger: %v", err)
	}

	structuredDocs := observation.StructuredDocumentObservationAdapter{
		Artifacts: []observation.DocumentArtifact{
			{
				ID:         "doc-payslip",
				UserID:     "user-1",
				Kind:       observation.DocumentKindPayslip,
				Filename:   "payslip_2026-03.csv",
				MediaType:  "text/csv",
				Content:    mustReadStateFixture(t, "payslip_2026-03.csv"),
				ObservedAt: now,
			},
			{
				ID:         "doc-broker",
				UserID:     "user-1",
				Kind:       observation.DocumentKindBrokerStatement,
				Filename:   "broker_statement_2026-03.csv",
				MediaType:  "text/csv",
				Content:    mustReadStateFixture(t, "broker_statement_2026-03.csv"),
				ObservedAt: now,
			},
		},
	}
	docEvidence, err := structuredDocs.Observe(t.Context(), observation.ObservationRequest{
		TaskID:     "task-monthly-review",
		SourceKind: "document",
		Params:     map[string]string{"user_id": "user-1"},
	})
	if err != nil {
		t.Fatalf("observe documents: %v", err)
	}

	agenticDocs := observation.AgenticDocumentObservationAdapterStub{
		Artifacts: []observation.DocumentArtifact{
			{
				ID:         "doc-tax",
				UserID:     "user-1",
				Kind:       observation.DocumentKindTaxDocument,
				Filename:   "tax_2026.txt",
				MediaType:  "text/plain",
				Content:    mustReadStateFixture(t, "tax_2026.txt"),
				ObservedAt: now,
			},
		},
	}
	taxEvidence, err := agenticDocs.Observe(t.Context(), observation.ObservationRequest{
		TaskID:     "task-monthly-review",
		SourceKind: "document",
		Params:     map[string]string{"user_id": "user-1"},
	})
	if err != nil {
		t.Fatalf("observe tax document: %v", err)
	}

	allEvidence := append(append(ledgerEvidence, docEvidence...), taxEvidence...)
	engine := reducers.DeterministicReducerEngine{
		Now: func() time.Time { return now },
	}
	patch, err := engine.BuildPatch(state.FinancialWorldState{
		UserID: "user-1",
	}, allEvidence, "task-monthly-review", "workflow-monthly-001", "observed")
	if err != nil {
		t.Fatalf("build evidence patch: %v", err)
	}

	next, diff, err := state.DefaultStateReducer{}.ApplyEvidencePatch(state.FinancialWorldState{UserID: "user-1"}, patch)
	if err != nil {
		t.Fatalf("apply patch: %v", err)
	}
	if next.CashflowState.MonthlyInflowCents != 800000 {
		t.Fatalf("expected monthly inflow 800000, got %d", next.CashflowState.MonthlyInflowCents)
	}
	if next.CashflowState.MonthlyOutflowCents != 155100 {
		t.Fatalf("expected monthly outflow 155100, got %d", next.CashflowState.MonthlyOutflowCents)
	}
	if next.CashflowState.SavingsRate <= 0.8 {
		t.Fatalf("expected savings rate above 0.8, got %.4f", next.CashflowState.SavingsRate)
	}
	if next.LiabilityState.TotalDebtCents != 750000 {
		t.Fatalf("expected total debt 750000, got %d", next.LiabilityState.TotalDebtCents)
	}
	if next.PortfolioState.AllocationDrift["equity"] <= 0 {
		t.Fatalf("expected positive equity drift, got %+v", next.PortfolioState.AllocationDrift)
	}
	if !next.TaxState.ChildcareTaxSignal {
		t.Fatalf("expected childcare tax signal to be true")
	}
	if next.BehaviorState.LateNightSpendingFrequency <= 0 {
		t.Fatalf("expected late-night spending frequency to be positive")
	}
	if next.BehaviorState.DuplicateSubscriptionCount != 2 {
		t.Fatalf("expected duplicate subscription count 2, got %d", next.BehaviorState.DuplicateSubscriptionCount)
	}
	if next.RiskState.OverallRisk == "" {
		t.Fatalf("expected overall risk to be set")
	}
	if diff.ToVersion != 1 {
		t.Fatalf("expected first version to be 1, got %+v", diff)
	}
}

func mustReadStateFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "tests", "fixtures", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}
