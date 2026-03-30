package taskspec

import (
	"strings"
	"time"
)

type TaskIntakeConfidence string

const (
	TaskIntakeConfidenceLow    TaskIntakeConfidence = "low"
	TaskIntakeConfidenceMedium TaskIntakeConfidence = "medium"
	TaskIntakeConfidenceHigh   TaskIntakeConfidence = "high"
)

type TaskIntakeFailureReason string

const (
	TaskIntakeFailureNone             TaskIntakeFailureReason = ""
	TaskIntakeFailureUnsupportedInput TaskIntakeFailureReason = "unsupported_input"
	TaskIntakeFailureMissingGoal      TaskIntakeFailureReason = "missing_goal"
	TaskIntakeFailureValidation       TaskIntakeFailureReason = "validation_failure"
)

type TaskRejectionReason string

const (
	TaskRejectionNone              TaskRejectionReason = ""
	TaskRejectionOutOfScope        TaskRejectionReason = "out_of_scope"
	TaskRejectionMissingIntent     TaskRejectionReason = "missing_intent"
	TaskRejectionInsufficientInput TaskRejectionReason = "insufficient_input"
)

type TaskIntakeResult struct {
	Accepted        bool                    `json:"accepted"`
	RawInput        string                  `json:"raw_input"`
	Confidence      TaskIntakeConfidence    `json:"confidence"`
	FailureReason   TaskIntakeFailureReason `json:"failure_reason,omitempty"`
	RejectionReason TaskRejectionReason     `json:"rejection_reason,omitempty"`
	Notes           []string                `json:"notes,omitempty"`
	TaskSpec        *TaskSpec               `json:"task_spec,omitempty"`
}

type DeterministicIntakeService struct {
	Now func() time.Time
}

func (s DeterministicIntakeService) Parse(raw string) TaskIntakeResult {
	input := strings.TrimSpace(raw)
	if input == "" {
		return TaskIntakeResult{
			Accepted:        false,
			RawInput:        raw,
			Confidence:      TaskIntakeConfidenceLow,
			FailureReason:   TaskIntakeFailureMissingGoal,
			RejectionReason: TaskRejectionInsufficientInput,
			Notes:           []string{"input is empty after trimming"},
		}
	}

	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now().UTC()
	}

	lower := strings.ToLower(input)
	switch {
	case isMonthlyReviewIntent(lower):
		spec := buildMonthlyReviewTaskSpec(input, now)
		if err := spec.Validate(); err != nil {
			return TaskIntakeResult{
				Accepted:      false,
				RawInput:      raw,
				Confidence:    TaskIntakeConfidenceMedium,
				FailureReason: TaskIntakeFailureValidation,
				Notes:         []string{err.Error()},
			}
		}
		return TaskIntakeResult{
			Accepted:   true,
			RawInput:   raw,
			Confidence: TaskIntakeConfidenceHigh,
			Notes: []string{
				"deterministic monthly review intent matched",
				"success criteria template injected",
			},
			TaskSpec: &spec,
		}
	case isDebtVsInvestIntent(lower):
		spec := buildDebtVsInvestTaskSpec(input, now)
		if err := spec.Validate(); err != nil {
			return TaskIntakeResult{
				Accepted:      false,
				RawInput:      raw,
				Confidence:    TaskIntakeConfidenceMedium,
				FailureReason: TaskIntakeFailureValidation,
				Notes:         []string{err.Error()},
			}
		}
		return TaskIntakeResult{
			Accepted:   true,
			RawInput:   raw,
			Confidence: TaskIntakeConfidenceHigh,
			Notes: []string{
				"deterministic debt-vs-invest intent matched",
				"evidence-backed decision requirements injected",
			},
			TaskSpec: &spec,
		}
	case isBehaviorInterventionIntent(lower):
		spec := buildBehaviorInterventionTaskSpec(input, now)
		if err := spec.Validate(); err != nil {
			return TaskIntakeResult{
				Accepted:      false,
				RawInput:      raw,
				Confidence:    TaskIntakeConfidenceMedium,
				FailureReason: TaskIntakeFailureValidation,
				Notes:         []string{err.Error()},
			}
		}
		return TaskIntakeResult{
			Accepted:   true,
			RawInput:   raw,
			Confidence: TaskIntakeConfidenceHigh,
			Notes: []string{
				"deterministic behavior intervention intent matched",
				"behavior skill-selection success criteria injected",
			},
			TaskSpec: &spec,
		}
	default:
		return TaskIntakeResult{
			Accepted:        false,
			RawInput:        raw,
			Confidence:      TaskIntakeConfidenceLow,
			FailureReason:   TaskIntakeFailureUnsupportedInput,
			RejectionReason: TaskRejectionOutOfScope,
			Notes:           []string{"deterministic intake only accepts monthly review, debt-vs-invest, and behavior intervention intents"},
		}
	}
}

func isMonthlyReviewIntent(input string) bool {
	keywords := []string{
		"月度财务复盘",
		"月度复盘",
		"财务复盘",
		"monthly financial review",
		"monthly review",
	}
	return containsAny(input, keywords)
}

func isDebtVsInvestIntent(input string) bool {
	keywords := []string{
		"提前还贷",
		"继续投资",
		"还贷还是投资",
		"debt vs invest",
		"pay down debt",
		"invest",
	}
	return containsAny(input, keywords)
}

func isBehaviorInterventionIntent(input string) bool {
	keywords := []string{
		"行为干预",
		"支出行为复盘",
		"消费习惯复盘",
		"订阅清理",
		"深夜消费",
		"消费护栏",
		"behavior intervention",
		"spending behavior review",
	}
	return containsAny(input, keywords)
}

func containsAny(input string, keywords []string) bool {
	for _, keyword := range keywords {
		if strings.Contains(input, strings.ToLower(keyword)) || strings.Contains(input, keyword) {
			return true
		}
	}
	return false
}

func buildMonthlyReviewTaskSpec(goal string, now time.Time) TaskSpec {
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0).Add(-time.Second)
	deadline := now.Add(48 * time.Hour)
	return TaskSpec{
		ID:    "task-monthly-review-" + now.Format("20060102"),
		Goal:  goal,
		Scope: TaskScope{Areas: []string{"cashflow", "liability", "portfolio", "tax", "behavior"}, Start: &start, End: &end},
		Constraints: ConstraintSet{
			Hard: []string{
				"all financial calculations must be deterministic",
				"high-risk recommendations cannot auto-execute",
			},
			Soft: []string{"prefer evidence-backed optimization suggestions"},
		},
		RiskLevel: RiskLevelMedium,
		SuccessCriteria: []SuccessCriteria{
			{ID: "coverage", Description: "required evidence for cashflow, liabilities, portfolio, and tax is covered"},
			{ID: "report", Description: "structured monthly report contains summary, risks, suggestions, and todo items"},
			{ID: "traceability", Description: "every major finding is backed by evidence or state-derived metrics"},
		},
		RequiredEvidence: []RequiredEvidenceRef{
			{Type: "transaction_batch", Reason: "cashflow metrics and spending behavior", Mandatory: true},
			{Type: "debt_obligation_snapshot", Reason: "debt pressure and minimum payment analysis", Mandatory: true},
			{Type: "portfolio_allocation_snapshot", Reason: "allocation drift and liquidity view", Mandatory: true},
			{Type: "payslip_statement", Reason: "income and payroll tax signals", Mandatory: false},
			{Type: "tax_document", Reason: "family-related tax opportunities", Mandatory: false},
		},
		ApprovalRequirement: ApprovalRequirementRecommended,
		Deadline:            &deadline,
		UserIntentType:      UserIntentMonthlyReview,
		CreatedAt:           now,
	}
}

func buildDebtVsInvestTaskSpec(goal string, now time.Time) TaskSpec {
	start := now.AddDate(0, -3, 0)
	deadline := now.Add(72 * time.Hour)
	return TaskSpec{
		ID:    "task-debt-vs-invest-" + now.Format("20060102"),
		Goal:  goal,
		Scope: TaskScope{Areas: []string{"cashflow", "liability", "portfolio"}, Start: &start, End: &now},
		Constraints: ConstraintSet{
			Hard: []string{
				"do not auto-execute any high-risk investment recommendation",
				"preserve liquidity coverage requirements",
			},
			Soft: []string{"prefer conclusions with explicit confidence and risk explanation"},
		},
		RiskLevel: RiskLevelHigh,
		SuccessCriteria: []SuccessCriteria{
			{ID: "comparison", Description: "compare debt paydown and investment continuation using deterministic metrics"},
			{ID: "risk", Description: "include liquidity impact and risk explanation"},
			{ID: "evidence", Description: "final conclusion is evidence-backed"},
		},
		RequiredEvidence: []RequiredEvidenceRef{
			{Type: "transaction_batch", Reason: "cashflow and liquidity baseline", Mandatory: true},
			{Type: "debt_obligation_snapshot", Reason: "debt obligations and minimum payment pressure", Mandatory: true},
			{Type: "portfolio_allocation_snapshot", Reason: "investable asset mix and drift", Mandatory: true},
		},
		ApprovalRequirement: ApprovalRequirementMandatory,
		Deadline:            &deadline,
		UserIntentType:      UserIntentDebtVsInvest,
		CreatedAt:           now,
	}
}

func buildBehaviorInterventionTaskSpec(goal string, now time.Time) TaskSpec {
	start := now.AddDate(0, -2, 0)
	deadline := now.Add(48 * time.Hour)
	return TaskSpec{
		ID:    "task-behavior-intervention-" + now.Format("20060102"),
		Goal:  goal,
		Scope: TaskScope{Areas: []string{"behavior", "cashflow"}, Start: &start, End: &now},
		Constraints: ConstraintSet{
			Hard: []string{
				"behavior recommendations must stay grounded in deterministic behavior metrics",
				"high-risk behavior interventions cannot auto-execute without governance approval",
			},
			Soft: []string{"prefer procedural-memory-aware skill selection when prior interventions exist"},
		},
		RiskLevel: RiskLevelHigh,
		SuccessCriteria: []SuccessCriteria{
			{ID: "anomaly_surfaced", Description: "behavior anomalies and trends are surfaced from deterministic evidence and state"},
			{ID: "skill_selection_explainable", Description: "selected skill family/version/recipe is explainable via evidence, state, and procedural memory"},
			{ID: "recommendation_grounded_governed", Description: "behavior recommendations are grounded, validated, and governed"},
		},
		RequiredEvidence: []RequiredEvidenceRef{
			{Type: "transaction_batch", Reason: "deterministic discretionary-spend analysis baseline", Mandatory: true},
			{Type: "recurring_subscription_signal", Reason: "subscription overlap and cleanup detection", Mandatory: false},
			{Type: "late_night_spending_signal", Reason: "late-night discretionary spend anomaly detection", Mandatory: false},
		},
		ApprovalRequirement: ApprovalRequirementRecommended,
		Deadline:            &deadline,
		UserIntentType:      UserIntentBehaviorIntervention,
		CreatedAt:           now,
	}
}
