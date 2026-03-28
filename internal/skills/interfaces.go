package skills

import (
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

type TriggerCondition struct {
	IntentType taskspec.UserIntentType `json:"intent_type"`
	Keywords   []string                `json:"keywords,omitempty"`
}

type Skill interface {
	Name() string
	Trigger() TriggerCondition
	RequiredContext() []string
	SuccessCriteriaTemplate() []taskspec.SuccessCriteria
	OutputContract() string
}

type MonthlyReviewSkillContract interface{ Skill }
type DebtOptimizationSkillContract interface{ Skill }
type TaxOptimizationSkillContract interface{ Skill }
type RebalanceSkillContract interface{ Skill }
type BehaviorInterventionSkillContract interface{ Skill }
