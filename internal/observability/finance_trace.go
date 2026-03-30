package observability

import "github.com/kobelakers/personal-cfo-os/internal/verification"

func FilterVerificationResultsByCategory(results []verification.VerificationResult, category verification.ValidationCategory) []verification.VerificationResult {
	filtered := make([]verification.VerificationResult, 0)
	for _, result := range results {
		if result.Category == category {
			filtered = append(filtered, result)
		}
	}
	return filtered
}

func PolicyRuleHitsFromDecisions(decisions []PolicyDecisionRecord) []PolicyRuleHitRecord {
	records := make([]PolicyRuleHitRecord, 0)
	for _, decision := range decisions {
		for _, ref := range decision.PolicyRuleRefs {
			if ref == "" {
				continue
			}
			records = append(records, PolicyRuleHitRecord{
				DecisionID:    decision.ID,
				RuleRef:       ref,
				Outcome:       decision.Outcome,
				Reason:        decision.Reason,
				OccurredAt:    decision.OccurredAt,
				CorrelationID: decision.CorrelationID,
			})
		}
	}
	return records
}

func ApprovalTriggersFromDecisions(decisions []PolicyDecisionRecord) []ApprovalTriggerRecord {
	records := make([]ApprovalTriggerRecord, 0)
	for _, decision := range decisions {
		if decision.Outcome != "require_approval" {
			continue
		}
		records = append(records, ApprovalTriggerRecord{
			Action:         decision.Action,
			Resource:       decision.Resource,
			Outcome:        decision.Outcome,
			Reason:         decision.Reason,
			PolicyRuleRefs: append([]string{}, decision.PolicyRuleRefs...),
			OccurredAt:     decision.OccurredAt,
			CorrelationID:  decision.CorrelationID,
		})
	}
	return records
}
