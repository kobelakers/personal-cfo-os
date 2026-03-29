package observability

type RuntimeReplayTrace struct {
	Scope             string          `json:"scope"`
	ScopeID           string          `json:"scope_id"`
	Provenance        ProvenanceChain `json:"provenance"`
	Events            []ReplayEvent   `json:"events"`
	Attempts          int             `json:"attempts"`
	RetryEvents       int             `json:"retry_events"`
	ApprovalActions   int             `json:"approval_actions"`
	CommittedAdvances int             `json:"committed_advances"`
	CurrentSummary    string          `json:"current_summary,omitempty"`
	ActionTypes       []string        `json:"action_types,omitempty"`
}
