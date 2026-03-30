package eval

import (
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/observability"
)

type RegressionFailure struct {
	ScenarioID string   `json:"scenario_id"`
	Code       string   `json:"code"`
	Message    string   `json:"message"`
	Expected   []string `json:"expected,omitempty"`
	Actual     []string `json:"actual,omitempty"`
}

type EvalResult struct {
	ScenarioID           string                         `json:"scenario_id"`
	Category             string                         `json:"category"`
	Description          string                         `json:"description"`
	Deterministic        bool                           `json:"deterministic"`
	Passed               bool                           `json:"passed"`
	RuntimeState         string                         `json:"runtime_state"`
	Scope                observability.ReplayScope      `json:"scope"`
	WorkflowID           string                         `json:"workflow_id,omitempty"`
	TaskGraphID          string                         `json:"task_graph_id,omitempty"`
	TaskID               string                         `json:"task_id,omitempty"`
	ExecutionID          string                         `json:"execution_id,omitempty"`
	ApprovalID           string                         `json:"approval_id,omitempty"`
	Replay               observability.ReplayView       `json:"replay"`
	DebugSummary         observability.DebugSummary     `json:"debug_summary"`
	Comparison           *observability.ReplayComparison `json:"comparison,omitempty"`
	TokenUsage           int                            `json:"token_usage"`
	EvidenceComplete     bool                           `json:"evidence_complete"`
	ChildWorkflowCount   int                            `json:"child_workflow_count"`
	RegressionFailures   []RegressionFailure            `json:"regression_failures,omitempty"`
	StartedAt            time.Time                      `json:"started_at"`
	CompletedAt          time.Time                      `json:"completed_at"`
	DurationMilliseconds int64                          `json:"duration_milliseconds"`
}

type EvalScore struct {
	ScenarioCount                int     `json:"scenario_count"`
	PassedCount                  int     `json:"passed_count"`
	FailedCount                  int     `json:"failed_count"`
	TaskSuccessRate              float64 `json:"task_success_rate"`
	ValidatorPassRate            float64 `json:"validator_pass_rate"`
	PolicyViolationRate          float64 `json:"policy_violation_rate"`
	ApprovalFrequency            float64 `json:"approval_frequency"`
	RetryFrequency               float64 `json:"retry_frequency"`
	AverageLatencyMilliseconds   float64 `json:"average_latency_milliseconds"`
	TotalTokenUsage              int     `json:"total_token_usage"`
	EvidenceCompletenessRate     float64 `json:"evidence_completeness_rate"`
	ChildWorkflowCompletionRate  float64 `json:"child_workflow_completion_rate"`
}

type EvalSummary struct {
	CorpusID          string   `json:"corpus_id"`
	DeterministicOnly bool     `json:"deterministic_only"`
	PassedScenarios   []string `json:"passed_scenarios,omitempty"`
	FailedScenarios   []string `json:"failed_scenarios,omitempty"`
	ApprovalScenarios []string `json:"approval_scenarios,omitempty"`
	SummaryLines      []string `json:"summary_lines,omitempty"`
}

type EvalRun struct {
	RunID             string        `json:"run_id"`
	CorpusID          string        `json:"corpus_id"`
	DeterministicOnly bool          `json:"deterministic_only"`
	StartedAt         time.Time     `json:"started_at"`
	CompletedAt       time.Time     `json:"completed_at"`
	Results           []EvalResult  `json:"results"`
	Score             EvalScore     `json:"score"`
	Summary           EvalSummary   `json:"summary"`
}

type EvalDiff struct {
	LeftRunID   string               `json:"left_run_id"`
	RightRunID  string               `json:"right_run_id"`
	Differences []ScenarioDiff       `json:"differences,omitempty"`
	Summary     []string             `json:"summary,omitempty"`
}

type ScenarioDiff struct {
	ScenarioID string   `json:"scenario_id"`
	Fields     []string `json:"fields,omitempty"`
	Summary    string   `json:"summary"`
}
