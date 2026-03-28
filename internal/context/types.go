package context

import "github.com/kobelakers/personal-cfo-os/internal/observation"

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

type InjectedStateBlock struct {
	Name      string `json:"name"`
	Version   uint64 `json:"version"`
	DataJSON  string `json:"data_json"`
	Source    string `json:"source"`
	Mandatory bool   `json:"mandatory"`
}

type ContextSlice struct {
	View           ContextView              `json:"view"`
	TaskID         string                   `json:"task_id"`
	Goal           string                   `json:"goal"`
	EvidenceIDs    []observation.EvidenceID `json:"evidence_ids,omitempty"`
	MemoryIDs      []string                 `json:"memory_ids,omitempty"`
	StateBlocks    []InjectedStateBlock     `json:"state_blocks,omitempty"`
	RequiredSkills []string                 `json:"required_skills,omitempty"`
	Compacted      bool                     `json:"compacted"`
}
