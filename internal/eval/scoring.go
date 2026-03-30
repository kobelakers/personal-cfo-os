package eval

import "strings"

func buildScore(results []EvalResult) EvalScore {
	score := EvalScore{ScenarioCount: len(results)}
	if len(results) == 0 {
		return score
	}
	var (
		successes          int
		validatorPasses    int
		policyViolations   int
		approvals          int
		retries            int
		evidenceComplete   int
		childComplete      int
		totalChildExpected int
		totalLatency       int64
	)
	for _, item := range results {
		if item.Passed {
			score.PassedCount++
		} else {
			score.FailedCount++
		}
		if item.RuntimeState == "completed" || item.RuntimeState == "waiting_approval" {
			successes++
		}
		if len(item.Replay.Summary.ValidatorSummary) == 0 && len(item.Replay.Explanation.WhyValidationFailed) == 0 {
			validatorPasses++
		}
		if hasPolicyViolation(item) {
			policyViolations++
		}
		if item.ApprovalID != "" || item.RuntimeState == "waiting_approval" {
			approvals++
		}
		if hasRetry(item) {
			retries++
		}
		if item.EvidenceComplete {
			evidenceComplete++
		}
		if item.ChildWorkflowCount > 0 {
			totalChildExpected += item.ChildWorkflowCount
			childComplete += len(item.Replay.Summary.ChildWorkflowSummary)
		}
		score.TotalTokenUsage += item.TokenUsage
		totalLatency += item.DurationMilliseconds
	}
	total := float64(len(results))
	score.TaskSuccessRate = float64(successes) / total
	score.ValidatorPassRate = float64(validatorPasses) / total
	score.PolicyViolationRate = float64(policyViolations) / total
	score.ApprovalFrequency = float64(approvals) / total
	score.RetryFrequency = float64(retries) / total
	score.AverageLatencyMilliseconds = float64(totalLatency) / total
	score.EvidenceCompletenessRate = float64(evidenceComplete) / total
	if totalChildExpected > 0 {
		score.ChildWorkflowCompletionRate = float64(childComplete) / float64(totalChildExpected)
	}
	return score
}

func hasPolicyViolation(result EvalResult) bool {
	for _, item := range result.Replay.FailureAttributions {
		if strings.Contains(item.FailureCategory, "governance") || strings.Contains(item.FailureCategory, "validation") {
			return true
		}
	}
	return false
}

func hasRetry(result EvalResult) bool {
	for _, item := range result.Replay.ExecutionAttributions {
		if item.Details["last_recovery_strategy"] == "retry" {
			return true
		}
	}
	return false
}
