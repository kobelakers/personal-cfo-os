package prompt

type PromptRenderPolicy struct {
	ContextInjectionPolicy string `json:"context_injection_policy,omitempty"`
}

const (
	ContextInjectionPolicyContextThenCandidateCatalog = "context_then_candidate_catalog"
	ContextInjectionPolicyContextThenGroundedMetrics  = "context_then_grounded_metrics"
)

type PromptTraceInput struct {
	SelectedStateBlocks  []string `json:"selected_state_blocks,omitempty"`
	SelectedMemoryIDs    []string `json:"selected_memory_ids,omitempty"`
	SelectedEvidenceIDs  []string `json:"selected_evidence_ids,omitempty"`
	SelectedSkillNames   []string `json:"selected_skill_names,omitempty"`
	ExcludedBlockRefs    []string `json:"excluded_block_refs,omitempty"`
	CompactionDecisions  []string `json:"compaction_decisions,omitempty"`
	EstimatedInputTokens int      `json:"estimated_input_tokens,omitempty"`
}
