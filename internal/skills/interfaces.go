package skills

import "github.com/kobelakers/personal-cfo-os/internal/taskspec"

type TriggerCondition struct {
	IntentType taskspec.UserIntentType `json:"intent_type"`
	Keywords   []string                `json:"keywords,omitempty"`
}

type Skill interface {
	Name() string
	Trigger() TriggerCondition
}

type MonthlyReviewSkill interface{ Skill }
type DebtOptimizationSkill interface{ Skill }
type TaxOptimizationSkill interface{ Skill }
type RebalanceSkill interface{ Skill }
type BehaviorInterventionSkill interface{ Skill }
