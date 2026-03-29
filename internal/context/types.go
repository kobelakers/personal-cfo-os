package context

import (
	"github.com/kobelakers/personal-cfo-os/internal/memory"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
)

type ContextView string

const (
	ContextViewPlanning     ContextView = "planning"
	ContextViewExecution    ContextView = "execution"
	ContextViewVerification ContextView = "verification"
)

type CompactionStrategy string

const (
	CompactionStrategyNone             CompactionStrategy = "none"
	CompactionStrategyStateAware       CompactionStrategy = "state_aware"
	CompactionStrategyEvidenceFocused  CompactionStrategy = "evidence_focused"
	CompactionStrategyVerificationLean CompactionStrategy = "verification_lean"
)

type ContextSelectionReason string

const (
	ContextSelectionRequiredEvidence ContextSelectionReason = "required_evidence"
	ContextSelectionRiskSignal       ContextSelectionReason = "risk_signal"
	ContextSelectionRecentState      ContextSelectionReason = "recent_state"
	ContextSelectionMemoryRelevance  ContextSelectionReason = "memory_relevance"
	ContextSelectionSkillRequirement ContextSelectionReason = "skill_requirement"
	ContextSelectionVerificationGap  ContextSelectionReason = "verification_gap"
)

type ContextBlockSource string

const (
	ContextBlockSourceTask     ContextBlockSource = "task"
	ContextBlockSourceState    ContextBlockSource = "state"
	ContextBlockSourceEvidence ContextBlockSource = "evidence"
	ContextBlockSourceMemory   ContextBlockSource = "memory"
	ContextBlockSourceSkill    ContextBlockSource = "skill"
)

type ContextBudget struct {
	MaxStateBlocks       int `json:"max_state_blocks"`
	MaxMemoryBlocks      int `json:"max_memory_blocks"`
	MaxEvidenceItems     int `json:"max_evidence_items"`
	MaxCharacters        int `json:"max_characters"`
	MaxInputTokens       int `json:"max_input_tokens,omitempty"`
	ReservedOutputTokens int `json:"reserved_output_tokens,omitempty"`
	HardTokenLimit       int `json:"hard_token_limit,omitempty"`
}

type TokenBudget struct {
	MaxInputTokens       int `json:"max_input_tokens,omitempty"`
	ReservedOutputTokens int `json:"reserved_output_tokens,omitempty"`
	HardTokenLimit       int `json:"hard_token_limit,omitempty"`
}

type ContextBlockDecision struct {
	Source          ContextBlockSource `json:"source"`
	Ref             string             `json:"ref"`
	EstimatedTokens int                `json:"estimated_tokens"`
	Reason          string             `json:"reason"`
}

type ContextBudgetDecision struct {
	EstimatedInputTokens int                    `json:"estimated_input_tokens,omitempty"`
	TargetInputTokens    int                    `json:"target_input_tokens,omitempty"`
	Included             []ContextBlockDecision `json:"included,omitempty"`
	Excluded             []ContextBlockDecision `json:"excluded,omitempty"`
}

type ContextCompactionResult struct {
	Strategy             CompactionStrategy `json:"strategy"`
	InitialTokenEstimate int                `json:"initial_token_estimate,omitempty"`
	FinalTokenEstimate   int                `json:"final_token_estimate,omitempty"`
	Notes                []string           `json:"notes,omitempty"`
}

type InjectedStateBlock struct {
	Name            string                 `json:"name"`
	Version         uint64                 `json:"version"`
	DataJSON        string                 `json:"data_json"`
	Source          string                 `json:"source"`
	Mandatory       bool                   `json:"mandatory"`
	BlockSource     ContextBlockSource     `json:"block_source"`
	SelectionReason ContextSelectionReason `json:"selection_reason"`
}

type MemoryBlock struct {
	MemoryID        string                 `json:"memory_id"`
	Kind            memory.MemoryKind      `json:"kind"`
	Summary         string                 `json:"summary"`
	BlockSource     ContextBlockSource     `json:"block_source"`
	SelectionReason ContextSelectionReason `json:"selection_reason"`
}

type EvidenceSummaryBlock struct {
	EvidenceID      observation.EvidenceID   `json:"evidence_id"`
	Type            observation.EvidenceType `json:"type"`
	Summary         string                   `json:"summary"`
	BlockSource     ContextBlockSource       `json:"block_source"`
	SelectionReason ContextSelectionReason   `json:"selection_reason"`
}

type SkillBlock struct {
	SkillName       string                 `json:"skill_name"`
	Description     string                 `json:"description"`
	BlockSource     ContextBlockSource     `json:"block_source"`
	SelectionReason ContextSelectionReason `json:"selection_reason"`
}

type ContextSlice struct {
	View           ContextView              `json:"view"`
	TaskID         string                   `json:"task_id"`
	Goal           string                   `json:"goal"`
	Budget         ContextBudget            `json:"budget"`
	TokenBudget    TokenBudget              `json:"token_budget,omitempty"`
	BudgetDecision ContextBudgetDecision    `json:"budget_decision,omitempty"`
	Compaction     ContextCompactionResult  `json:"compaction,omitempty"`
	EvidenceIDs    []observation.EvidenceID `json:"evidence_ids,omitempty"`
	MemoryIDs      []string                 `json:"memory_ids,omitempty"`
	StateBlocks    []InjectedStateBlock     `json:"state_blocks,omitempty"`
	MemoryBlocks   []MemoryBlock            `json:"memory_blocks,omitempty"`
	EvidenceBlocks []EvidenceSummaryBlock   `json:"evidence_blocks,omitempty"`
	SkillBlocks    []SkillBlock             `json:"skill_blocks,omitempty"`
	RequiredSkills []string                 `json:"required_skills,omitempty"`
	Compacted      bool                     `json:"compacted"`
}
