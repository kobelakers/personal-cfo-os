package verification

import (
	"testing"

	"github.com/kobelakers/personal-cfo-os/internal/analysis"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
)

func TestRunCashflowGroundingPrecheckRejectsUnsupportedNumericClaim(t *testing.T) {
	candidate := validCashflowCandidate()
	candidate.Summary = "本月净结余为 700000 分，现金流依然可控。"

	err := RunCashflowGroundingPrecheck(candidate, validCashflowMetrics(), []observation.EvidenceID{"evidence-1"})
	if err == nil {
		t.Fatalf("expected unsupported numeric claim to fail grounding")
	}
}

func TestRunCashflowGroundingPrecheckRejectsMetricSpecificMismatch(t *testing.T) {
	candidate := validCashflowCandidate()
	candidate.Summary = "本月流入 644900 分，整体可控。"

	err := RunCashflowGroundingPrecheck(candidate, validCashflowMetrics(), []observation.EvidenceID{"evidence-1"})
	if err == nil {
		t.Fatalf("expected metric-specific mismatch to fail grounding")
	}
}

func TestRunCashflowGroundingPrecheckAcceptsGroundedCandidate(t *testing.T) {
	if err := RunCashflowGroundingPrecheck(validCashflowCandidate(), validCashflowMetrics(), []observation.EvidenceID{"evidence-1"}); err != nil {
		t.Fatalf("expected grounded candidate to pass: %v", err)
	}
}

func validCashflowCandidate() analysis.CashflowStructuredCandidate {
	return analysis.CashflowStructuredCandidate{
		Summary:     "本月净结余为 644900 分，储蓄率 0.81，整体可控。",
		KeyFindings: []string{"重复订阅 2 个，值得继续清理。", "深夜消费频率 0.12，建议继续观察。"},
		GroundedRecommendations: []analysis.Recommendation{
			{
				Type:          analysis.RecommendationTypeExpenseReduction,
				Title:         "优先清理 2 个重复订阅",
				Detail:        "当前净结余 644900 分，先优化订阅类可变支出。",
				RiskLevel:     "low",
				GroundingRefs: []string{"metric:duplicate_subscription_count"},
				EvidenceRefs:  []string{"evidence-1"},
			},
		},
		RiskFlags: []analysis.RiskFlag{
			{
				Code:        "late_night_spending",
				Severity:    "low",
				Detail:      "深夜消费频率 0.12，建议继续观察。",
				EvidenceIDs: []observation.EvidenceID{"evidence-1"},
			},
		},
		MetricRefs:   []string{"monthly_net_income_cents", "savings_rate", "duplicate_subscription_count", "late_night_spending_frequency"},
		EvidenceRefs: []string{"evidence-1"},
		Confidence:   0.82,
		Caveats:      []string{"所有金额与比率仍以 deterministic metrics 为准。"},
	}
}

func validCashflowMetrics() analysis.CashflowDeterministicMetrics {
	return analysis.CashflowDeterministicMetrics{
		MonthlyInflowCents:         800000,
		MonthlyOutflowCents:        155100,
		MonthlyNetIncomeCents:      644900,
		SavingsRate:                0.81,
		DuplicateSubscriptionCount: 2,
		LateNightSpendingFrequency: 0.12,
	}
}
