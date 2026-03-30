package eval

import (
	"context"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/observability"
	"github.com/kobelakers/personal-cfo-os/internal/runtime"
)

type ScenarioExpectation struct {
	ExpectedFinalState       string   `json:"expected_final_state,omitempty"`
	ExpectedScopeKind        string   `json:"expected_scope_kind,omitempty"`
	RequireApproval          bool     `json:"require_approval,omitempty"`
	RequireComparison        bool     `json:"require_comparison,omitempty"`
	RequiredComparisonCategories []string `json:"required_comparison_categories,omitempty"`
	RequireFailureExplanation bool    `json:"require_failure_explanation,omitempty"`
	RequireValidatorSummary  bool     `json:"require_validator_summary,omitempty"`
	MinimumChildWorkflows    int      `json:"minimum_child_workflows,omitempty"`
	RequiredProvenanceEdges  []string `json:"required_provenance_edges,omitempty"`
}

type ScenarioCase struct {
	ID            string              `json:"id"`
	Category      string              `json:"category"`
	Description   string              `json:"description"`
	Deterministic bool                `json:"deterministic"`
	Expectation   ScenarioExpectation `json:"expectation"`
	run           scenarioRunner
}

type ScenarioCorpus struct {
	ID                string         `json:"id"`
	DeterministicOnly bool           `json:"deterministic_only"`
	Cases             []ScenarioCase `json:"cases"`
}

type scenarioRunner func(context.Context, ScenarioRunContext) (scenarioRunOutput, error)

type ScenarioRunContext struct {
	FixtureDir string
	WorkDir    string
	Now        time.Time
}

type scenarioRunOutput struct {
	RuntimeState       runtime.WorkflowExecutionState
	WorkflowID         string
	TaskGraphID        string
	TaskID             string
	ExecutionID        string
	ApprovalID         string
	Replay             observability.ReplayView
	DebugSummary       observability.DebugSummary
	Comparison         *observability.ReplayComparison
	TokenUsage         int
	EvidenceComplete   bool
	ChildWorkflowCount int
}
