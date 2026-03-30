package eval

import "fmt"

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
			diffs = append(diffs, ScenarioDiff{ScenarioID: id, Fields: []string{"scenario_presence"}, Summary: "scenario only exists in right run"})
		case !rightOK:
			diffs = append(diffs, ScenarioDiff{ScenarioID: id, Fields: []string{"scenario_presence"}, Summary: "scenario only exists in left run"})
		default:
			fields := make([]string, 0)
			if leftItem.Passed != rightItem.Passed {
				fields = append(fields, "passed")
			}
			if leftItem.RuntimeState != rightItem.RuntimeState {
				fields = append(fields, "runtime_state")
			}
			if leftItem.Scope.Kind != rightItem.Scope.Kind || leftItem.Scope.ID != rightItem.Scope.ID {
				fields = append(fields, "scope")
			}
			if len(leftItem.Replay.Summary.PlanSummary) != len(rightItem.Replay.Summary.PlanSummary) {
				fields = append(fields, "plan_summary")
			}
			if len(leftItem.Replay.Summary.MemorySummary) != len(rightItem.Replay.Summary.MemorySummary) {
				fields = append(fields, "memory_summary")
			}
			if len(leftItem.Replay.Summary.ValidatorSummary) != len(rightItem.Replay.Summary.ValidatorSummary) {
				fields = append(fields, "validator_summary")
			}
			if len(leftItem.Replay.Summary.GovernanceSummary) != len(rightItem.Replay.Summary.GovernanceSummary) {
				fields = append(fields, "governance_summary")
			}
			if len(leftItem.Replay.Summary.ChildWorkflowSummary) != len(rightItem.Replay.Summary.ChildWorkflowSummary) {
				fields = append(fields, "child_workflow_summary")
			}
			if leftItem.TokenUsage != rightItem.TokenUsage {
				fields = append(fields, "token_usage")
			}
			if len(fields) > 0 {
				diffs = append(diffs, ScenarioDiff{
					ScenarioID: id,
					Fields:     fields,
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
