package behavior

import (
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/finance"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/skills"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

// Layer 7 + Layer 10: behavior is a first-class domain with deterministic
// evidence, typed state, explicit recommendations, and validator semantics.

type BehaviorEvidence struct {
	CurrentState state.FinancialWorldState   `json:"current_state"`
	Evidence     []observation.EvidenceRecord `json:"evidence,omitempty"`
}

type BehaviorStateView struct {
	Cashflow state.CashflowState `json:"cashflow"`
	Behavior state.BehaviorState `json:"behavior"`
	Risk     state.RiskState     `json:"risk"`
}

type BehaviorMetricBundle struct {
	Metrics BehaviorMetrics       `json:"metrics"`
	Records []finance.MetricRecord `json:"records,omitempty"`
}

type BehaviorMetrics struct {
	DuplicateSubscriptionCount        int     `json:"duplicate_subscription_count"`
	LateNightSpendCount               int     `json:"late_night_spend_count"`
	LateNightSpendRatio               float64 `json:"late_night_spend_ratio"`
	DiscretionaryPressureScore        float64 `json:"discretionary_pressure_score"`
	RecurringSubscriptionCount        int     `json:"recurring_subscription_count"`
	MonthlyVariableExpenseCents       int64   `json:"monthly_variable_expense_cents"`
	MonthlyNetIncomeCents             int64   `json:"monthly_net_income_cents"`
}

type BehaviorTrend struct {
	Code   string `json:"code"`
	Detail string `json:"detail"`
}

type BehaviorAnomaly struct {
	Code         string   `json:"code"`
	Severity     string   `json:"severity"`
	Detail       string   `json:"detail"`
	MetricRefs   []string `json:"metric_refs,omitempty"`
	EvidenceRefs []string `json:"evidence_refs,omitempty"`
}

type BehaviorRecommendationType string

const (
	BehaviorRecommendationSubscriptionCleanup BehaviorRecommendationType = "subscription_cleanup"
	BehaviorRecommendationSpendNudge          BehaviorRecommendationType = "spend_nudge"
	BehaviorRecommendationGuardrail           BehaviorRecommendationType = "guardrail"
)

type BehaviorRiskLevel = taskspec.RiskLevel

type BehaviorRecommendation struct {
	Type             BehaviorRecommendationType `json:"type"`
	Title            string                     `json:"title"`
	Detail           string                     `json:"detail"`
	RiskLevel        BehaviorRiskLevel          `json:"risk_level"`
	EvidenceRefs     []string                   `json:"evidence_refs,omitempty"`
	MetricRefs       []string                   `json:"metric_refs,omitempty"`
	StateRefs        []string                   `json:"state_refs,omitempty"`
	MemoryRefs       []string                   `json:"memory_refs,omitempty"`
	Caveats          []string                   `json:"caveats,omitempty"`
	ApprovalRequired bool                       `json:"approval_required,omitempty"`
	ApprovalReason   string                     `json:"approval_reason,omitempty"`
	PolicyRuleRefs   []string                   `json:"policy_rule_refs,omitempty"`
}

type AnalysisOutput struct {
	Summary         string                   `json:"summary"`
	Metrics         BehaviorMetricBundle     `json:"metrics"`
	Trends          []BehaviorTrend          `json:"trends,omitempty"`
	Anomalies       []BehaviorAnomaly        `json:"anomalies,omitempty"`
	Recommendations []BehaviorRecommendation `json:"recommendations,omitempty"`
	SelectedSkill   skills.SkillSelection    `json:"selected_skill"`
	EvidenceRefs    []string                 `json:"evidence_refs,omitempty"`
	MemoryRefs      []string                 `json:"memory_refs,omitempty"`
	GeneratedAt     time.Time                `json:"generated_at"`
}

type BehaviorValidator struct{}
