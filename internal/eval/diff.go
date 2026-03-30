package eval

import (
	"fmt"

	"github.com/kobelakers/personal-cfo-os/internal/observability"
)

func CompareRuns(left EvalRun, right EvalRun) EvalDiff {
	leftByID := make(map[string]EvalResult, len(left.Results))
	for _, item := range left.Results {
		leftByID[item.ScenarioID] = item
	}
	rightByID := make(map[string]EvalResult, len(right.Results))
	for _, item := range right.Results {
		rightByID[item.ScenarioID] = item
	}

	diffs := make([]ScenarioDiff, 0)
	summary := make([]string, 0)
	seen := make(map[string]struct{}, len(leftByID)+len(rightByID))
	for id := range leftByID {
		seen[id] = struct{}{}
	}
	for id := range rightByID {
		seen[id] = struct{}{}
	}
	for id := range seen {
		leftItem, leftOK := leftByID[id]
		rightItem, rightOK := rightByID[id]
		switch {
		case !leftOK:
			diffs = append(diffs, ScenarioDiff{
				ScenarioID: id,
				Fields:     []string{"scenario_presence"},
				Details:    []string{"left=(missing)", "right=present"},
				Summary:    "scenario only exists in right run",
			})
		case !rightOK:
			diffs = append(diffs, ScenarioDiff{
				ScenarioID: id,
				Fields:     []string{"scenario_presence"},
				Details:    []string{"left=present", "right=(missing)"},
				Summary:    "scenario only exists in left run",
			})
		default:
			fields := make([]string, 0)
			details := make([]string, 0)
			if leftItem.Passed != rightItem.Passed {
				fields = append(fields, "passed")
				details = append(details, fmt.Sprintf("passed: left=%t right=%t", leftItem.Passed, rightItem.Passed))
			}
			if leftItem.RuntimeState != rightItem.RuntimeState {
				fields = append(fields, "runtime_state")
				details = append(details, fmt.Sprintf("runtime_state: left=%s right=%s", leftItem.RuntimeState, rightItem.RuntimeState))
			}
			if leftItem.Scope.Kind != rightItem.Scope.Kind || leftItem.Scope.ID != rightItem.Scope.ID {
				fields = append(fields, "scope")
				details = append(details, fmt.Sprintf("scope: left=%s:%s right=%s:%s", leftItem.Scope.Kind, leftItem.Scope.ID, rightItem.Scope.Kind, rightItem.Scope.ID))
			}
			comparison := observability.BuildReplayComparison(leftItem.Replay, rightItem.Replay)
			for _, diff := range comparison.Diffs {
				fields = append(fields, diff.Field)
				details = append(details, fmt.Sprintf("%s: %s", diff.Category, diff.Summary))
				details = append(details, diff.Details...)
			}
			if leftItem.TokenUsage != rightItem.TokenUsage {
				fields = append(fields, "token_usage")
				details = append(details, fmt.Sprintf("token_usage: left=%d right=%d", leftItem.TokenUsage, rightItem.TokenUsage))
			}
			if len(fields) > 0 {
				diffs = append(diffs, ScenarioDiff{
					ScenarioID: id,
					Fields:     fields,
					Details:    details,
					Summary:    fmt.Sprintf("scenario %s changed across eval runs", id),
				})
			}
		}
	}
	for _, item := range diffs {
		summary = append(summary, item.Summary)
	}
	return EvalDiff{
		LeftRunID:   left.RunID,
		RightRunID:  right.RunID,
		Differences: diffs,
		Summary:     summary,
	}
}
