package skills

import "time"

// Layer 7: Skills + Tools Layer.
// Skills are versioned, policy-aware capabilities, not prompt fragments.

type SkillFamily string
type SkillVersion string
type SkillExecutionStatus string
type InterventionIntensity string

const (
	SkillFamilySubscriptionCleanup  SkillFamily = "subscription_cleanup"
	SkillFamilyLateNightSpendNudge  SkillFamily = "late_night_spend_nudge"
	SkillFamilyDiscretionaryGuardrail SkillFamily = "discretionary_guardrail"
)

const (
	InterventionIntensityLow      InterventionIntensity = "low"
	InterventionIntensityModerate InterventionIntensity = "moderate"
	InterventionIntensityHigh     InterventionIntensity = "high"
)

const (
	SkillExecutionStatusSelected  SkillExecutionStatus = "selected"
	SkillExecutionStatusExecuted  SkillExecutionStatus = "executed"
	SkillExecutionStatusFailed    SkillExecutionStatus = "failed"
	SkillExecutionStatusGoverned  SkillExecutionStatus = "governed"
)

type SkillManifest struct {
	Family         SkillFamily          `json:"family"`
	Version        SkillVersion         `json:"version"`
	Title          string               `json:"title"`
	Description    string               `json:"description"`
	Trigger        SkillTrigger         `json:"trigger"`
	Policy         SkillPolicy          `json:"policy"`
	ExpectedOutput SkillExpectedOutput  `json:"expected_output"`
	Recipes        []SkillRecipe        `json:"recipes"`
}

type SkillTrigger struct {
	Intent           string   `json:"intent"`
	Keywords         []string `json:"keywords,omitempty"`
	RequiredEvidence []string `json:"required_evidence,omitempty"`
}

type SkillPolicy struct {
	RequiresApprovalRecipes []string `json:"requires_approval_recipes,omitempty"`
	PolicyRuleRefs          []string `json:"policy_rule_refs,omitempty"`
}

type SkillExpectedOutput struct {
	BlockKind           string   `json:"block_kind"`
	RecommendationTypes []string `json:"recommendation_types,omitempty"`
}

type SkillRecipe struct {
	ID                    string                `json:"id"`
	Title                 string                `json:"title"`
	Description           string                `json:"description"`
	InterventionIntensity InterventionIntensity `json:"intervention_intensity"`
	RequiredEvidenceTypes []string              `json:"required_evidence_types,omitempty"`
	RequiredStateBlocks   []string              `json:"required_state_blocks,omitempty"`
	ApprovalRequired      bool                  `json:"approval_required,omitempty"`
	PolicyRuleRefs        []string              `json:"policy_rule_refs,omitempty"`
}

type SkillSelectionReason struct {
	Code           string   `json:"code"`
	Detail         string   `json:"detail"`
	EvidenceRefs   []string `json:"evidence_refs,omitempty"`
	StateRefs      []string `json:"state_refs,omitempty"`
	MemoryRefs     []string `json:"memory_refs,omitempty"`
	PolicyRuleRefs []string `json:"policy_rule_refs,omitempty"`
}

type SkillSelection struct {
	Family                SkillFamily             `json:"family"`
	Version               SkillVersion            `json:"version"`
	RecipeID              string                  `json:"recipe_id"`
	Reasons               []SkillSelectionReason  `json:"reasons,omitempty"`
	EvidenceRefs          []string                `json:"evidence_refs,omitempty"`
	StateRefs             []string                `json:"state_refs,omitempty"`
	MemoryRefs            []string                `json:"memory_refs,omitempty"`
	PolicyRuleRefs        []string                `json:"policy_rule_refs,omitempty"`
	InterventionIntensity InterventionIntensity   `json:"intervention_intensity"`
}

type SkillExecutionRecord struct {
	WorkflowID         string               `json:"workflow_id,omitempty"`
	TaskID             string               `json:"task_id,omitempty"`
	ExecutionID        string               `json:"execution_id,omitempty"`
	Selection          SkillSelection       `json:"selection"`
	Status             SkillExecutionStatus `json:"status"`
	ValidatorOutcome   string               `json:"validator_outcome,omitempty"`
	GovernanceOutcome  string               `json:"governance_outcome,omitempty"`
	ProducedArtifactIDs []string            `json:"produced_artifact_ids,omitempty"`
	ProducedMemoryRefs []string             `json:"produced_memory_refs,omitempty"`
	CreatedAt          time.Time            `json:"created_at"`
	UpdatedAt          time.Time            `json:"updated_at"`
}

// ProceduralMemoryContextRecord is the stable memory influence surface consumed
// by the skill selector. Keeping it in the skills layer avoids coupling the
// selector to memory-store internals while still letting procedural memory
// influence capability choice.
type ProceduralMemoryContextRecord struct {
	ID    string            `json:"id"`
	Kind  string            `json:"kind"`
	Facts map[string]string `json:"facts,omitempty"`
}
