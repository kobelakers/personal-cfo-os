package taskspec

import "time"

type RiskLevel string

const (
	RiskLevelUnknown  RiskLevel = "unknown"
	RiskLevelLow      RiskLevel = "low"
	RiskLevelMedium   RiskLevel = "medium"
	RiskLevelHigh     RiskLevel = "high"
	RiskLevelCritical RiskLevel = "critical"
)

type ApprovalRequirement string

const (
	ApprovalRequirementNone        ApprovalRequirement = "none"
	ApprovalRequirementNotify      ApprovalRequirement = "notify"
	ApprovalRequirementRecommended ApprovalRequirement = "recommended"
	ApprovalRequirementMandatory   ApprovalRequirement = "mandatory"
)

type UserIntentType string

const (
	UserIntentUnknown            UserIntentType = "unknown"
	UserIntentMonthlyReview      UserIntentType = "monthly_review"
	UserIntentDebtVsInvest       UserIntentType = "debt_vs_invest"
	UserIntentLifeEventTrigger   UserIntentType = "life_event_trigger"
	UserIntentTaxOptimization    UserIntentType = "tax_optimization"
	UserIntentPortfolioRebalance UserIntentType = "portfolio_rebalance"
)

type TaskScope struct {
	Areas []string   `json:"areas"`
	Start *time.Time `json:"start,omitempty"`
	End   *time.Time `json:"end,omitempty"`
	Notes []string   `json:"notes,omitempty"`
}

type ConstraintSet struct {
	Hard []string `json:"hard"`
	Soft []string `json:"soft,omitempty"`
}

type SuccessCriteria struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}

type RequiredEvidenceRef struct {
	Type      string `json:"type"`
	Reason    string `json:"reason"`
	Mandatory bool   `json:"mandatory"`
}

type TaskSpec struct {
	ID                  string                `json:"id"`
	Goal                string                `json:"goal"`
	Scope               TaskScope             `json:"scope"`
	Constraints         ConstraintSet         `json:"constraints"`
	RiskLevel           RiskLevel             `json:"risk_level"`
	SuccessCriteria     []SuccessCriteria     `json:"success_criteria"`
	RequiredEvidence    []RequiredEvidenceRef `json:"required_evidence"`
	ApprovalRequirement ApprovalRequirement   `json:"approval_requirement"`
	Deadline            *time.Time            `json:"deadline,omitempty"`
	UserIntentType      UserIntentType        `json:"user_intent_type"`
	CreatedAt           time.Time             `json:"created_at"`
}
