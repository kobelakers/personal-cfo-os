package observability

import (
	"fmt"
	"strings"

	"github.com/kobelakers/personal-cfo-os/internal/verification"
)

func BuildReplaySummaryFromTrace(trace WorkflowTraceDump, finalState string) ReplaySummary {
	return ReplaySummary{
		PlanSummary:          tracePlanSummary(trace),
		SkillSummary:         traceSkillSummary(trace),
		MemorySummary:        traceMemorySummary(trace),
		ValidatorSummary:     traceValidatorSummary(trace),
		GovernanceSummary:    traceGovernanceSummary(trace),
		ChildWorkflowSummary: traceChildWorkflowSummary(trace),
		FinalState:           finalState,
	}
}

func BuildReplayExplanationFromTrace(trace WorkflowTraceDump, finalState string) ReplayExplanation {
	explanation := ReplayExplanation{
		WhySkillSelected:    traceSkillSelectionExplanation(trace),
		WhyGeneratedTask:    traceEventSummary(trace, []string{"generated_task_graph"}),
		WhyChildExecuted:    traceEventSummary(trace, []string{"life_event_follow_up_execution", "follow_up_execution"}),
		WhyMemoryDecision:   traceMemoryDecisionExplanation(trace),
		WhyValidationFailed: traceValidationFailureExplanation(trace),
	}
	if finalState == "failed" {
		explanation.WhyFailed = strings.Join(traceValidationFailureExplanation(trace), "; ")
		if explanation.WhyFailed == "" {
			explanation.WhyFailed = firstNonEmptyTraceEvent(trace, []string{"failure", "failure_handled"})
		}
	}
	if finalState == "waiting_approval" {
		explanation.WhyWaitingApproval = firstNonEmptyTraceEvent(trace, []string{"waiting_approval", "approval_required"})
		if explanation.WhyWaitingApproval == "" && len(trace.ApprovalTriggers) > 0 {
			explanation.WhyWaitingApproval = trace.ApprovalTriggers[len(trace.ApprovalTriggers)-1].Reason
		}
	}
	return explanation
}

func BuildDebugSummaryFromTrace(workflowID string, trace WorkflowTraceDump, finalState string) DebugSummary {
	summary := BuildReplaySummaryFromTrace(trace, finalState)
	explanation := BuildReplayExplanationFromTrace(trace, finalState)
	lines := make([]string, 0, 2)
	if explanation.WhyFailed != "" {
		lines = append(lines, explanation.WhyFailed)
	}
	if explanation.WhyWaitingApproval != "" {
		lines = append(lines, explanation.WhyWaitingApproval)
	}
	lines = append(lines, explanation.WhySkillSelected...)
	lines = append(lines, explanation.WhyValidationFailed...)
	lines = append(lines, explanation.WhyMemoryDecision...)
	lines = dedupeStrings(lines)
	return DebugSummary{
		WorkflowID:        workflowID,
		FinalRuntimeState: finalState,
		PlanSummary:       summary.PlanSummary,
		SkillSummary:      summary.SkillSummary,
		MemorySummary:     summary.MemorySummary,
		ValidatorSummary:  summary.ValidatorSummary,
		GovernanceSummary: summary.GovernanceSummary,
		ChildWorkflows:    summary.ChildWorkflowSummary,
		Explanation:       lines,
	}
}

func BuildDebugSummaryFromReplay(view ReplayView) DebugSummary {
	explanation := make([]string, 0, 2)
	if view.Explanation.WhyFailed != "" {
		explanation = append(explanation, view.Explanation.WhyFailed)
	}
	if view.Explanation.WhyWaitingApproval != "" {
		explanation = append(explanation, view.Explanation.WhyWaitingApproval)
	}
	explanation = append(explanation, view.Explanation.WhySkillSelected...)
	explanation = append(explanation, view.Explanation.WhyValidationFailed...)
	explanation = append(explanation, view.Explanation.WhyMemoryDecision...)
	summary := DebugSummary{
		FinalRuntimeState: view.Summary.FinalState,
		PlanSummary:       append([]string{}, view.Summary.PlanSummary...),
		SkillSummary:      append([]string{}, view.Summary.SkillSummary...),
		MemorySummary:     append([]string{}, view.Summary.MemorySummary...),
		ValidatorSummary:  append([]string{}, view.Summary.ValidatorSummary...),
		GovernanceSummary: append([]string{}, view.Summary.GovernanceSummary...),
		ChildWorkflows:    append([]string{}, view.Summary.ChildWorkflowSummary...),
		Explanation:       dedupeStrings(explanation),
	}
	if view.Workflow != nil {
		summary.WorkflowID = view.Workflow.WorkflowID
		summary.Goal = firstNonEmpty(view.Summary.GoalSummary, view.Workflow.Summary)
	}
	if view.TaskGraph != nil {
		summary.TaskGraphID = view.TaskGraph.TaskGraphID
	}
	return summary
}

func traceSkillSummary(trace WorkflowTraceDump) []string {
	result := make([]string, 0)
	for _, item := range trace.Events {
		switch item.Category {
		case "skill_selected":
			result = append(result, fmt.Sprintf("selected=%s/%s", item.Details["skill_family"], item.Details["recipe_id"]))
			if details := strings.TrimSpace(item.Details["reason_details"]); details != "" {
				result = append(result, fmt.Sprintf("selection_reasons=%s", details))
			}
			if refs := strings.TrimSpace(item.Details["memory_refs"]); refs != "" {
				result = append(result, fmt.Sprintf("selection_memory_refs=%s", refs))
			}
		case "skill_execution":
			result = append(result, fmt.Sprintf("executed=%s/%s", item.Details["skill_family"], item.Details["recipe_id"]))
			if rules := strings.TrimSpace(item.Details["policy_rule_refs"]); rules != "" {
				result = append(result, fmt.Sprintf("skill_policy_refs=%s", rules))
			}
		case "skill_outcome_memory_written":
			result = append(result, fmt.Sprintf("outcome_memory=%s", item.Details["memory_ids"]))
			if state := strings.TrimSpace(item.Details["final_runtime_state"]); state != "" {
				result = append(result, fmt.Sprintf("skill_outcome_state=%s", state))
			}
		}
	}
	return dedupeStrings(result)
}

func traceSkillSelectionExplanation(trace WorkflowTraceDump) []string {
	result := make([]string, 0)
	for _, item := range trace.Events {
		switch item.Category {
		case "skill_selected":
			if details := strings.TrimSpace(item.Details["reason_details"]); details != "" {
				result = append(result, fmt.Sprintf("skill selected because %s", details))
			} else if strings.TrimSpace(item.Message) != "" {
				result = append(result, item.Message)
			}
			if refs := strings.TrimSpace(item.Details["memory_refs"]); refs != "" {
				result = append(result, fmt.Sprintf("procedural memory influenced selection: %s", refs))
			}
		case "skill_outcome_memory_written":
			if ids := strings.TrimSpace(item.Details["memory_ids"]); ids != "" {
				result = append(result, fmt.Sprintf("skill outcome memory written: %s", ids))
			}
		}
	}
	return dedupeStrings(result)
}

func tracePlanSummary(trace WorkflowTraceDump) []string {
	result := make([]string, 0)
	for _, item := range trace.AgentExecutions {
		if item.Recipient != "planner_agent" || len(item.PlanBlockIDs) == 0 {
			continue
		}
		result = append(result, fmt.Sprintf("%s blocks=%s", firstNonEmpty(item.ResultSummary, "planner execution"), strings.Join(item.PlanBlockIDs, ",")))
	}
	return dedupeStrings(result)
}

func traceMemorySummary(trace WorkflowTraceDump) []string {
	result := make([]string, 0)
	for _, item := range trace.MemorySelections {
		if len(item.SelectedMemoryIDs) > 0 {
			result = append(result, fmt.Sprintf("selected=%s", strings.Join(item.SelectedMemoryIDs, ",")))
		}
		if len(item.RejectedMemoryIDs) > 0 {
			result = append(result, fmt.Sprintf("rejected=%s", strings.Join(item.RejectedMemoryIDs, ",")))
		}
	}
	if len(result) == 0 {
		for _, item := range trace.MemoryRetrievals {
			if len(item.SelectedMemoryID) > 0 {
				result = append(result, fmt.Sprintf("selected=%s", strings.Join(item.SelectedMemoryID, ",")))
			}
			if len(item.RejectedMemoryID) > 0 {
				result = append(result, fmt.Sprintf("rejected=%s", strings.Join(item.RejectedMemoryID, ",")))
			}
			for _, candidate := range item.Results {
				if !candidate.Rejected || candidate.RejectionRule == "" {
					continue
				}
				result = append(result, fmt.Sprintf("rejection_rule=%s:%s", candidate.MemoryID, candidate.RejectionRule))
			}
		}
	} else {
		for _, item := range trace.MemoryRetrievals {
			for _, candidate := range item.Results {
				if !candidate.Rejected || candidate.RejectionRule == "" {
					continue
				}
				result = append(result, fmt.Sprintf("rejection_rule=%s:%s", candidate.MemoryID, candidate.RejectionRule))
			}
		}
	}
	return dedupeStrings(result)
}

func traceValidatorSummary(trace WorkflowTraceDump) []string {
	result := make([]string, 0)
	appendResults := func(label string, items []verification.VerificationResult) {
		for _, item := range items {
			result = append(result, fmt.Sprintf("%s:%s:%s", label, item.Validator, item.Status))
		}
	}
	appendResults("grounding", trace.GroundingVerdicts)
	appendResults("numeric", trace.NumericValidationVerdicts)
	appendResults("business", trace.BusinessRuleVerdicts)
	return dedupeStrings(result)
}

func traceGovernanceSummary(trace WorkflowTraceDump) []string {
	result := make([]string, 0)
	for _, item := range trace.PolicyDecisions {
		result = append(result, fmt.Sprintf("%s:%s", item.Action, item.Outcome))
	}
	for _, item := range trace.ApprovalTriggers {
		result = append(result, fmt.Sprintf("%s:%s", item.Action, item.Outcome))
	}
	return dedupeStrings(result)
}

func traceChildWorkflowSummary(trace WorkflowTraceDump) []string {
	result := make([]string, 0)
	for _, item := range trace.Events {
		if item.Category != "life_event_follow_up_execution" && item.Category != "follow_up_execution" {
			continue
		}
		if taskIDs := item.Details["executed_task_ids"]; strings.TrimSpace(taskIDs) != "" {
			result = append(result, taskIDs)
			continue
		}
		if summary := strings.TrimSpace(item.Message); summary != "" {
			result = append(result, summary)
		}
	}
	return dedupeStrings(result)
}

func traceMemoryDecisionExplanation(trace WorkflowTraceDump) []string {
	result := make([]string, 0)
	for _, item := range trace.MemorySelections {
		if len(item.SelectedMemoryIDs) > 0 {
			result = append(result, fmt.Sprintf("memory selected: %s", strings.Join(item.SelectedMemoryIDs, ",")))
		}
		if len(item.RejectedMemoryIDs) > 0 {
			result = append(result, fmt.Sprintf("memory rejected: %s", strings.Join(item.RejectedMemoryIDs, ",")))
		}
		if strings.TrimSpace(item.Reason) != "" {
			result = append(result, item.Reason)
		}
	}
	for _, item := range trace.MemoryRetrievals {
		for _, candidate := range item.Results {
			if !candidate.Rejected {
				continue
			}
			switch {
			case candidate.RejectionRule != "" && candidate.RejectionReason != "":
				result = append(result, fmt.Sprintf("memory rejected by %s: %s (%s)", candidate.RejectionRule, candidate.MemoryID, candidate.RejectionReason))
			case candidate.RejectionRule != "":
				result = append(result, fmt.Sprintf("memory rejected by %s: %s", candidate.RejectionRule, candidate.MemoryID))
			case candidate.RejectionReason != "":
				result = append(result, fmt.Sprintf("memory rejected: %s (%s)", candidate.MemoryID, candidate.RejectionReason))
			}
		}
	}
	return dedupeStrings(result)
}

func traceValidationFailureExplanation(trace WorkflowTraceDump) []string {
	result := make([]string, 0)
	appendFailures := func(items []verification.VerificationResult) {
		for _, item := range items {
			if item.Status != verification.VerificationStatusFail && item.Status != verification.VerificationStatusNeedsReplan {
				continue
			}
			result = append(result, fmt.Sprintf("%s: %s", item.Validator, item.Message))
		}
	}
	appendFailures(trace.GroundingVerdicts)
	appendFailures(trace.NumericValidationVerdicts)
	appendFailures(trace.BusinessRuleVerdicts)
	return dedupeStrings(result)
}

func traceEventSummary(trace WorkflowTraceDump, eventTypes []string) []string {
	allowed := make(map[string]struct{}, len(eventTypes))
	for _, item := range eventTypes {
		allowed[item] = struct{}{}
	}
	result := make([]string, 0)
	for _, item := range trace.Events {
		if _, ok := allowed[item.Category]; !ok {
			continue
		}
		result = append(result, firstNonEmpty(item.Message, item.Category))
	}
	return dedupeStrings(result)
}

func firstNonEmptyTraceEvent(trace WorkflowTraceDump, eventTypes []string) string {
	items := traceEventSummary(trace, eventTypes)
	if len(items) == 0 {
		return ""
	}
	return items[len(items)-1]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}
