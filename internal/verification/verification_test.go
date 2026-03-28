package verification

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

func TestVerificationArtifactsRoundTrip(t *testing.T) {
	now := time.Date(2026, 3, 28, 13, 0, 0, 0, time.UTC)
	result := VerificationResult{
		Status:    VerificationStatusNeedsReplan,
		Validator: "evidence-coverage-checker",
		Message:   "Tax evidence is incomplete.",
		EvidenceCoverage: EvidenceCoverageReport{
			TaskID:        "task-1",
			CoverageRatio: 0.5,
			Items: []EvidenceCoverageItem{
				{RequirementID: "tax-document", Covered: false, GapReason: "No W-2 evidence supplied"},
				{RequirementID: "ledger", Covered: true, EvidenceIDs: []observation.EvidenceID{observation.EvidenceID("evidence-1")}},
			},
		},
		CheckedAt: now,
	}
	verdict := OracleVerdict{
		Scenario:  "missing-tax-evidence",
		Passed:    false,
		Score:     0.4,
		Reasons:   []string{"evidence gap remained unresolved"},
		CheckedAt: now,
	}

	resultData, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal verification result: %v", err)
	}
	var decodedResult VerificationResult
	if err := json.Unmarshal(resultData, &decodedResult); err != nil {
		t.Fatalf("unmarshal verification result: %v", err)
	}
	if err := decodedResult.Validate(); err != nil {
		t.Fatalf("decoded verification result should validate: %v", err)
	}

	verdictData, err := json.Marshal(verdict)
	if err != nil {
		t.Fatalf("marshal oracle verdict: %v", err)
	}
	var decodedVerdict OracleVerdict
	if err := json.Unmarshal(verdictData, &decodedVerdict); err != nil {
		t.Fatalf("unmarshal oracle verdict: %v", err)
	}
	if err := decodedVerdict.Validate(); err != nil {
		t.Fatalf("decoded oracle verdict should validate: %v", err)
	}
}

func TestEvidenceCoverageCheckerFailsWhenMandatoryEvidenceMissing(t *testing.T) {
	checker := DefaultEvidenceCoverageChecker{}
	spec := taskspec.TaskSpec{
		ID: "task-coverage",
		RequiredEvidence: []taskspec.RequiredEvidenceRef{
			{Type: "transaction_batch", Reason: "cashflow", Mandatory: true},
			{Type: "debt_obligation_snapshot", Reason: "debt", Mandatory: true},
		},
	}
	report, err := checker.Check(spec, []observation.EvidenceRecord{
		{
			ID:            "evidence-1",
			Type:          observation.EvidenceTypeTransactionBatch,
			Source:        observation.EvidenceSource{Kind: "ledger", Adapter: "test", Reference: "r1", Provenance: "p1"},
			TimeRange:     observation.EvidenceTimeRange{ObservedAt: time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)},
			Confidence:    observation.EvidenceConfidence{Score: 0.9, Reason: "test"},
			Claims:        []observation.EvidenceClaim{{Subject: "cashflow", Predicate: "monthly_inflow_cents", Object: "month", ValueJSON: "100000"}},
			Normalization: observation.EvidenceNormalizationResult{Status: observation.EvidenceNormalizationNormalized},
			Summary:       "batch",
			CreatedAt:     time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC),
		},
	})
	if err != nil {
		t.Fatalf("coverage check: %v", err)
	}
	result := coverageToVerificationResultForTest(spec, report)
	if result.Status != VerificationStatusNeedsReplan {
		t.Fatalf("expected needs replan for missing mandatory evidence, got %+v", result)
	}
}

func TestSuccessCriteriaCheckerFailsWhenValidatorFails(t *testing.T) {
	checker := DefaultSuccessCriteriaChecker{}
	spec := taskspec.TaskSpec{
		ID: "task-success",
		SuccessCriteria: []taskspec.SuccessCriteria{
			{ID: "report", Description: "output must be complete"},
		},
	}
	result, err := checker.Check(spec, []VerificationResult{
		{
			Status:           VerificationStatusFail,
			Validator:        "monthly_review_business_validator",
			Message:          "tax signal omitted",
			EvidenceCoverage: EvidenceCoverageReport{TaskID: spec.ID, CoverageRatio: 1},
			CheckedAt:        time.Now().UTC(),
		},
	}, map[string]any{"summary": "x"})
	if err != nil {
		t.Fatalf("success checker: %v", err)
	}
	if result.Status != VerificationStatusNeedsReplan {
		t.Fatalf("expected needs replan, got %+v", result)
	}
}

func TestMonthlyReviewBusinessValidatorFailsWhenTaxSignalMissing(t *testing.T) {
	validator := MonthlyReviewBusinessValidator{}
	spec := taskspec.TaskSpec{ID: "task-business"}
	current := state.FinancialWorldState{
		UserID: "user-1",
		TaxState: state.TaxState{
			ChildcareTaxSignal: true,
		},
	}
	result, err := validator.Validate(t.Context(), spec, current, nil, map[string]any{
		"summary":                  "monthly review complete",
		"risk_items":               []string{"现金流稳定"},
		"optimization_suggestions": []string{"压缩订阅支出"},
		"todo_items":               []string{"复查预算"},
		"cashflow_metrics":         map[string]any{"monthly_inflow_cents": 100000},
		"approval_required":        false,
	})
	if err != nil {
		t.Fatalf("business validator: %v", err)
	}
	if result.Status != VerificationStatusFail {
		t.Fatalf("expected fail when tax signal omitted, got %+v", result)
	}
}

func coverageToVerificationResultForTest(spec taskspec.TaskSpec, report EvidenceCoverageReport) VerificationResult {
	status := VerificationStatusPass
	for i, item := range report.Items {
		if !item.Covered && i < len(spec.RequiredEvidence) && spec.RequiredEvidence[i].Mandatory {
			status = VerificationStatusNeedsReplan
			break
		}
	}
	return VerificationResult{
		Status:           status,
		Validator:        "coverage-test",
		Message:          "coverage test",
		EvidenceCoverage: report,
		CheckedAt:        time.Now().UTC(),
	}
}
