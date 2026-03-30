package observability

import (
	"fmt"
	"slices"
	"strings"
)

func BuildReplayComparison(left ReplayView, right ReplayView) ReplayComparison {
	diffs := make([]ReplayComparisonDiff, 0)
	appendScalarDiff := func(category string, field string, leftValue string, rightValue string, summary string) {
		if leftValue == rightValue {
			return
		}
		diffs = append(diffs, ReplayComparisonDiff{
			Category: category,
			Field:    field,
			Left:     compactValues([]string{leftValue}),
			Right:    compactValues([]string{rightValue}),
			Details: []string{
				fmt.Sprintf("left=%s", blankAsNone(leftValue)),
				fmt.Sprintf("right=%s", blankAsNone(rightValue)),
			},
			Summary: summary,
		})
	}
	appendSectionDiff := func(category string, field string, leftValues []string, rightValues []string, summary string) {
		if slices.Equal(leftValues, rightValues) {
			return
		}
		added, removed := diffStringSlices(leftValues, rightValues)
		details := make([]string, 0, 4)
		if len(added) > 0 {
			details = append(details, fmt.Sprintf("added: %s", joinOrNone(added)))
		}
		if len(removed) > 0 {
			details = append(details, fmt.Sprintf("removed: %s", joinOrNone(removed)))
		}
		if len(added) == 0 && len(removed) == 0 {
			details = append(details, "content changed without pure add/remove delta")
		}
		details = append(details, fmt.Sprintf("left_count=%d", len(compactValues(leftValues))))
		details = append(details, fmt.Sprintf("right_count=%d", len(compactValues(rightValues))))
		diffs = append(diffs, ReplayComparisonDiff{
			Category: category,
			Field:    field,
			Left:     compactValues(leftValues),
			Right:    compactValues(rightValues),
			Details:  details,
			Summary:  summary,
		})
	}

	appendScalarDiff("runtime", "final_state", left.Summary.FinalState, right.Summary.FinalState, "final runtime state changed")
	appendSectionDiff("planning", "plan_summary", left.Summary.PlanSummary, right.Summary.PlanSummary, "planning summary changed")
	appendSectionDiff("memory", "memory_summary", left.Summary.MemorySummary, right.Summary.MemorySummary, "memory selection/rejection changed")
	appendSectionDiff("validation", "validator_summary", left.Summary.ValidatorSummary, right.Summary.ValidatorSummary, "validator verdicts changed")
	appendSectionDiff("governance", "governance_summary", left.Summary.GovernanceSummary, right.Summary.GovernanceSummary, "governance outcome changed")
	appendSectionDiff("runtime", "child_workflow_summary", left.Summary.ChildWorkflowSummary, right.Summary.ChildWorkflowSummary, "child workflow execution changed")

	summary := make([]string, 0, len(diffs))
	for _, diff := range diffs {
		summary = append(summary, diff.Summary)
	}
	return ReplayComparison{
		Left:    left.Scope,
		Right:   right.Scope,
		Diffs:   diffs,
		Summary: summary,
	}
}

func diffStringSlices(left []string, right []string) ([]string, []string) {
	leftValues := compactValues(left)
	rightValues := compactValues(right)
	added := make([]string, 0)
	for _, item := range rightValues {
		if !slices.Contains(leftValues, item) {
			added = append(added, item)
		}
	}
	removed := make([]string, 0)
	for _, item := range leftValues {
		if !slices.Contains(rightValues, item) {
			removed = append(removed, item)
		}
	}
	return added, removed
}

func compactValues(values []string) []string {
	result := make([]string, 0, len(values))
	for _, item := range values {
		if item == "" {
			continue
		}
		result = append(result, item)
	}
	return result
}

func joinOrNone(values []string) string {
	if len(values) == 0 {
		return "(none)"
	}
	return strings.Join(values, " | ")
}

func blankAsNone(value string) string {
	if value == "" {
		return "(none)"
	}
	return value
}
