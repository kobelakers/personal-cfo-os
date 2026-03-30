package verification

import (
	"testing"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/analysis"
	contextview "github.com/kobelakers/personal-cfo-os/internal/context"
	"github.com/kobelakers/personal-cfo-os/internal/finance"
	"github.com/kobelakers/personal-cfo-os/internal/memory"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

func TestFinancialGroundingValidatorRejectsUnsupportedMetricRef(t *testing.T) {
	validator := FinancialGroundingValidator{}
	payload := validTrustReportPayload()
	payload.MonthlyReview.Recommendations[0].MetricRefs = []string{"missing_metric_ref"}

	results, err := validator.Validate(
		t.Context(),
		taskspec.TaskSpec{ID: "task-1", UserIntentType: taskspec.UserIntentMonthlyReview},
		state.FinancialWorldState{},
		validTrustEvidence(),
		validTrustMemories(),
		validTrustVerificationContext(),
		payload,
	)
	if err != nil {
		t.Fatalf("grounding validate: %v", err)
	}
	if len(results) != 1 || results[0].Status != VerificationStatusFail {
		t.Fatalf("expected grounding failure, got %+v", results)
	}
	assertDiagnosticCode(t, results[0].Diagnostics, "unsupported_metric_ref")
}

func TestFinancialGroundingValidatorRejectsMissingCaveat(t *testing.T) {
	validator := FinancialGroundingValidator{}
	payload := validTrustReportPayload()
	payload.MonthlyReview.Recommendations[0].RiskLevel = taskspec.RiskLevelHigh
	payload.MonthlyReview.Recommendations[0].Caveats = nil

	results, err := validator.Validate(
		t.Context(),
		taskspec.TaskSpec{ID: "task-1", UserIntentType: taskspec.UserIntentMonthlyReview},
		state.FinancialWorldState{},
		validTrustEvidence(),
		validTrustMemories(),
		validTrustVerificationContext(),
		payload,
	)
	if err != nil {
		t.Fatalf("grounding validate: %v", err)
	}
	if len(results) != 1 || results[0].Status != VerificationStatusFail {
		t.Fatalf("expected missing caveat to fail, got %+v", results)
	}
	assertDiagnosticCode(t, results[0].Diagnostics, "missing_caveat")
}

func TestFinancialNumericConsistencyValidatorRejectsUnsupportedNumericClaim(t *testing.T) {
	validator := FinancialNumericConsistencyValidator{}
	payload := validTrustReportPayload()
	payload.MonthlyReview.Recommendations[0].Detail = "建议把月结余提升到 999999 分后再行动。"

	results, err := validator.Validate(
		t.Context(),
		taskspec.TaskSpec{ID: "task-1", UserIntentType: taskspec.UserIntentMonthlyReview},
		state.FinancialWorldState{},
		nil,
		nil,
		contextview.BlockVerificationContext{},
		payload,
	)
	if err != nil {
		t.Fatalf("numeric validate: %v", err)
	}
	if len(results) != 1 || results[0].Status != VerificationStatusFail {
		t.Fatalf("expected numeric failure, got %+v", results)
	}
	assertDiagnosticCode(t, results[0].Diagnostics, "unsupported_numeric_claim")
}

func TestTrustBusinessRuleValidatorRejectsAggressiveLowLiquidityRecommendationWithoutRisk(t *testing.T) {
	validator := TrustBusinessRuleValidator{}
	payload := validTrustReportPayload()
	payload.MonthlyReview.Recommendations[0].Type = analysis.RecommendationTypeInvestMore
	payload.MonthlyReview.Recommendations[0].RiskLevel = taskspec.RiskLevelMedium
	payload.MonthlyReview.Recommendations[0].Caveats = nil

	results, err := validator.Validate(
		t.Context(),
		taskspec.TaskSpec{ID: "task-1", UserIntentType: taskspec.UserIntentMonthlyReview},
		state.FinancialWorldState{},
		validTrustEvidence(),
		validTrustMemories(),
		validTrustVerificationContext(),
		payload,
	)
	if err != nil {
		t.Fatalf("business validate: %v", err)
	}
	if len(results) != 1 || results[0].Status != VerificationStatusFail {
		t.Fatalf("expected business-rule failure, got %+v", results)
	}
	assertDiagnosticCode(t, results[0].Diagnostics, "aggressive_low_liquidity_risk_missing")
	assertDiagnosticCode(t, results[0].Diagnostics, "aggressive_low_liquidity_caveat_missing")
}

func TestFinancialValidatorsPassGroundedRecommendation(t *testing.T) {
	payload := validTrustReportPayload()
	spec := taskspec.TaskSpec{ID: "task-1", UserIntentType: taskspec.UserIntentMonthlyReview}
	ctx := validTrustVerificationContext()

	grounding, err := FinancialGroundingValidator{}.Validate(t.Context(), spec, state.FinancialWorldState{}, validTrustEvidence(), validTrustMemories(), ctx, payload)
	if err != nil {
		t.Fatalf("grounding validate: %v", err)
	}
	numeric, err := FinancialNumericConsistencyValidator{}.Validate(t.Context(), spec, state.FinancialWorldState{}, nil, nil, ctx, payload)
	if err != nil {
		t.Fatalf("numeric validate: %v", err)
	}
	business, err := TrustBusinessRuleValidator{}.Validate(t.Context(), spec, state.FinancialWorldState{}, nil, nil, ctx, payload)
	if err != nil {
		t.Fatalf("business validate: %v", err)
	}
	if grounding[0].Status != VerificationStatusPass || numeric[0].Status != VerificationStatusPass || business[0].Status != VerificationStatusPass {
		t.Fatalf("expected fully grounded trust report to pass, got grounding=%+v numeric=%+v business=%+v", grounding, numeric, business)
	}
}

type fakeTrustReport struct {
	Recommendations []analysis.Recommendation
	RiskFlags       []analysis.RiskFlag
	MetricRecords   []finance.MetricRecord
}

type fakeTrustPayload struct {
	MonthlyReview *fakeTrustReport
}

func validTrustReportPayload() fakeTrustPayload {
	now := time.Date(2026, 3, 30, 8, 0, 0, 0, time.UTC)
	records := []finance.MetricRecord{
		{Ref: "emergency_fund_coverage_months", Domain: "cashflow", Name: "emergency_fund_coverage_months", ValueType: finance.MetricValueTypeFloat64, Float64Value: 2.2, Unit: "months", AsOf: now, EvidenceRefs: []string{"evidence-1"}},
		{Ref: "liquidity_buffer_months", Domain: "cashflow", Name: "liquidity_buffer_months", ValueType: finance.MetricValueTypeFloat64, Float64Value: 2.2, Unit: "months", AsOf: now, EvidenceRefs: []string{"evidence-1"}},
		{Ref: "monthly_net_income_cents", Domain: "cashflow", Name: "monthly_net_income_cents", ValueType: finance.MetricValueTypeInt64, Int64Value: 644900, Unit: "cents", AsOf: now, EvidenceRefs: []string{"evidence-1"}},
		{Ref: "debt_pressure_score", Domain: "cashflow", Name: "debt_pressure_score", ValueType: finance.MetricValueTypeFloat64, Float64Value: 0.52, Unit: "score", AsOf: now, EvidenceRefs: []string{"evidence-1"}},
	}
	return fakeTrustPayload{
		MonthlyReview: &fakeTrustReport{
			MetricRecords: records,
			Recommendations: []analysis.Recommendation{
				{
					ID:            "rec-1",
					Type:          analysis.RecommendationTypeExpenseReduction,
					Title:         "优先稳住 644900 分月结余",
					Detail:        "在流动性只有 2.2 个月的情况下，先削减可选支出，再决定是否扩大风险敞口。",
					RiskLevel:     taskspec.RiskLevelMedium,
					GroundingRefs: []string{"metric:monthly_net_income_cents", "metric:liquidity_buffer_months"},
					MetricRefs:    []string{"monthly_net_income_cents", "liquidity_buffer_months", "emergency_fund_coverage_months"},
					EvidenceRefs:  []string{"evidence-1"},
					MemoryRefs:    []string{"memory-1"},
					Caveats:       []string{"若后续考虑提高风险敞口，需先补足紧急备用金。"},
				},
			},
			RiskFlags: []analysis.RiskFlag{
				{
					Code:        "liquidity_buffer",
					Severity:    "medium",
					Detail:      "当前流动性缓冲约 2.2 个月，需要先稳住现金缓冲。",
					EvidenceIDs: []observation.EvidenceID{"evidence-1"},
					MetricRefs:  []string{"liquidity_buffer_months", "emergency_fund_coverage_months"},
					MemoryRefs:  []string{"memory-1"},
					Caveats:     []string{"流动性不足时不应直接给出激进投资建议。"},
				},
			},
		},
	}
}

func validTrustEvidence() []observation.EvidenceRecord {
	return []observation.EvidenceRecord{{ID: "evidence-1"}}
}

func validTrustMemories() []memory.MemoryRecord {
	return []memory.MemoryRecord{{ID: "memory-1"}}
}

func validTrustVerificationContext() contextview.BlockVerificationContext {
	return contextview.BlockVerificationContext{
		SelectedEvidenceIDs: []observation.EvidenceID{"evidence-1"},
		SelectedMemoryIDs:   []string{"memory-1"},
	}
}

func assertDiagnosticCode(t *testing.T, diagnostics []ValidationDiagnostic, code string) {
	t.Helper()
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return
		}
	}
	t.Fatalf("expected diagnostic code %q in %+v", code, diagnostics)
}
