package eval

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/observability"
)

type HarnessOptions struct {
	FixtureDir string
	WorkDir    string
	Now        func() time.Time
}

type Harness struct {
	fixtureDir string
	workDir    string
	now        func() time.Time
}

func NewHarness(options HarnessOptions) *Harness {
	nowFn := options.Now
	if nowFn == nil {
		nowFn = func() time.Time { return time.Now().UTC() }
	}
	return &Harness{
		fixtureDir: options.FixtureDir,
		workDir:    options.WorkDir,
		now:        nowFn,
	}
}

func (h *Harness) RunCorpus(ctx context.Context, corpus ScenarioCorpus) (EvalRun, error) {
	startedAt := h.now().UTC()
	results := make([]EvalResult, 0, len(corpus.Cases))
	for _, item := range corpus.Cases {
		result, err := h.RunScenario(ctx, item)
		if err != nil {
			return EvalRun{}, err
		}
		results = append(results, result)
	}
	score := buildScore(results)
	summary := buildSummary(corpus, results, score)
	return EvalRun{
		RunID:             fmt.Sprintf("%s-%d", corpus.ID, startedAt.UnixNano()),
		CorpusID:          corpus.ID,
		DeterministicOnly: corpus.DeterministicOnly,
		StartedAt:         startedAt,
		CompletedAt:       h.now().UTC(),
		Results:           results,
		Score:             score,
		Summary:           summary,
	}, nil
}

func (h *Harness) RunScenario(ctx context.Context, scenario ScenarioCase) (EvalResult, error) {
	if scenario.run == nil {
		return EvalResult{}, fmt.Errorf("scenario %q has no runner", scenario.ID)
	}
	startedAt := h.now().UTC()
	workDir, cleanup, err := h.prepareScenarioDir(scenario.ID)
	if err != nil {
		return EvalResult{}, err
	}
	defer cleanup()

	output, err := scenario.run(ctx, ScenarioRunContext{
		FixtureDir: h.fixtureDir,
		WorkDir:    workDir,
		Now:        startedAt,
	})
	if err != nil {
		return EvalResult{}, err
	}
	result := EvalResult{
		ScenarioID:           scenario.ID,
		Category:             scenario.Category,
		Description:          scenario.Description,
		Deterministic:        scenario.Deterministic,
		RuntimeState:         firstNonEmpty(string(output.RuntimeState), output.Replay.Summary.FinalState),
		WorkflowID:           output.WorkflowID,
		TaskGraphID:          output.TaskGraphID,
		TaskID:               output.TaskID,
		ExecutionID:          output.ExecutionID,
		ApprovalID:           output.ApprovalID,
		Scope:                output.Replay.Scope,
		Replay:               output.Replay,
		DebugSummary:         output.DebugSummary,
		Comparison:           output.Comparison,
		TokenUsage:           output.TokenUsage,
		EvidenceComplete:     output.EvidenceComplete,
		ChildWorkflowCount:   output.ChildWorkflowCount,
		StartedAt:            startedAt,
		CompletedAt:          h.now().UTC(),
	}
	result.DurationMilliseconds = result.CompletedAt.Sub(result.StartedAt).Milliseconds()
	result.RegressionFailures = validateExpectation(result, scenario.Expectation)
	result.Passed = len(result.RegressionFailures) == 0
	return result, nil
}

func buildSummary(corpus ScenarioCorpus, results []EvalResult, score EvalScore) EvalSummary {
	summary := EvalSummary{
		CorpusID:          corpus.ID,
		DeterministicOnly: corpus.DeterministicOnly,
		PassedScenarios:   make([]string, 0),
		FailedScenarios:   make([]string, 0),
		ApprovalScenarios: make([]string, 0),
	}
	for _, item := range results {
		if item.Passed {
			summary.PassedScenarios = append(summary.PassedScenarios, item.ScenarioID)
		} else {
			summary.FailedScenarios = append(summary.FailedScenarios, item.ScenarioID)
		}
		if item.ApprovalID != "" || item.RuntimeState == "waiting_approval" {
			summary.ApprovalScenarios = append(summary.ApprovalScenarios, item.ScenarioID)
		}
	}
	summary.SummaryLines = []string{
		fmt.Sprintf("corpus=%s deterministic_only=%t", corpus.ID, corpus.DeterministicOnly),
		fmt.Sprintf("passed=%d failed=%d", score.PassedCount, score.FailedCount),
		fmt.Sprintf("task_success_rate=%.2f validator_pass_rate=%.2f approval_frequency=%.2f", score.TaskSuccessRate, score.ValidatorPassRate, score.ApprovalFrequency),
	}
	return summary
}

func validateExpectation(result EvalResult, expectation ScenarioExpectation) []RegressionFailure {
	failures := make([]RegressionFailure, 0)
	appendFailure := func(code string, message string, expected []string, actual []string) {
		failures = append(failures, RegressionFailure{
			ScenarioID: result.ScenarioID,
			Code:       code,
			Message:    message,
			Expected:   expected,
			Actual:     actual,
		})
	}
	if expectation.ExpectedFinalState != "" && result.RuntimeState != expectation.ExpectedFinalState {
		appendFailure("unexpected_final_state", "scenario runtime state did not match expectation", []string{expectation.ExpectedFinalState}, []string{result.RuntimeState})
	}
	if expectation.ExpectedScopeKind != "" && result.Scope.Kind != expectation.ExpectedScopeKind {
		appendFailure("unexpected_scope_kind", "scenario replay scope kind did not match expectation", []string{expectation.ExpectedScopeKind}, []string{result.Scope.Kind})
	}
	if expectation.RequireApproval && result.ApprovalID == "" {
		appendFailure("missing_approval", "scenario expected approval metadata but none was recorded", []string{"approval_id"}, nil)
	}
	if expectation.RequireComparison && result.Comparison == nil {
		appendFailure("missing_comparison", "scenario expected a replay comparison but none was produced", []string{"comparison"}, nil)
	}
	for _, category := range expectation.RequiredComparisonCategories {
		if !hasComparisonCategory(result.Comparison, category) {
			appendFailure("missing_comparison_category", "scenario comparison is missing a required diff category", []string{category}, comparisonCategories(result.Comparison))
		}
	}
	if expectation.RequireFailureExplanation && result.Replay.Explanation.WhyFailed == "" && result.Replay.Explanation.WhyValidationFailed == nil {
		appendFailure("missing_failure_explanation", "scenario expected a structured failure explanation", []string{"why_failed or why_validation_failed"}, nil)
	}
	if expectation.RequireValidatorSummary && len(result.Replay.Summary.ValidatorSummary) == 0 {
		appendFailure("missing_validator_summary", "scenario expected validator summary entries", []string{"validator_summary"}, nil)
	}
	if expectation.MinimumChildWorkflows > 0 && result.ChildWorkflowCount < expectation.MinimumChildWorkflows {
		appendFailure("missing_child_workflows", "scenario expected child workflow executions", []string{fmt.Sprintf("%d", expectation.MinimumChildWorkflows)}, []string{fmt.Sprintf("%d", result.ChildWorkflowCount)})
	}
	for _, edgeType := range expectation.RequiredProvenanceEdges {
		if !hasEdgeType(result.Replay.Provenance, edgeType) {
			appendFailure("missing_provenance_edge", "scenario provenance graph is missing a required edge type", []string{edgeType}, provenanceEdgeTypes(result.Replay.Provenance))
		}
	}
	return failures
}

func (h *Harness) prepareScenarioDir(scenarioID string) (string, func(), error) {
	if h.workDir == "" {
		dir, err := os.MkdirTemp("", "personal-cfo-eval-"+scenarioID+"-*")
		if err != nil {
			return "", nil, err
		}
		return dir, func() { _ = os.RemoveAll(dir) }, nil
	}
	dir := filepath.Join(h.workDir, scenarioID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", nil, err
	}
	return dir, func() {}, nil
}

func hasEdgeType(graph observability.ProvenanceGraph, edgeType string) bool {
	for _, edge := range graph.Edges {
		if edge.Type == edgeType {
			return true
		}
	}
	return false
}

func provenanceEdgeTypes(graph observability.ProvenanceGraph) []string {
	types := make([]string, 0, len(graph.Edges))
	for _, edge := range graph.Edges {
		if edge.Type == "" || slices.Contains(types, edge.Type) {
			continue
		}
		types = append(types, edge.Type)
	}
	return types
}

func hasComparisonCategory(comparison *observability.ReplayComparison, category string) bool {
	if comparison == nil {
		return false
	}
	for _, diff := range comparison.Diffs {
		if diff.Category == category {
			return true
		}
	}
	return false
}

func comparisonCategories(comparison *observability.ReplayComparison) []string {
	if comparison == nil {
		return nil
	}
	categories := make([]string, 0, len(comparison.Diffs))
	for _, diff := range comparison.Diffs {
		if diff.Category == "" || slices.Contains(categories, diff.Category) {
			continue
		}
		categories = append(categories, diff.Category)
	}
	return categories
}

func firstNonEmpty(values ...string) string {
	for _, item := range values {
		if item != "" {
			return item
		}
	}
	return ""
}
