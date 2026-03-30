package governance

import "github.com/kobelakers/personal-cfo-os/internal/observability"

func ToObservabilityRecords(entries []AuditEvent) []observability.PolicyDecisionRecord {
	records := make([]observability.PolicyDecisionRecord, 0, len(entries))
	for _, entry := range entries {
		records = append(records, observability.PolicyDecisionRecord{
			ID:            entry.ID,
			Actor:         entry.Actor,
			Action:        entry.Action,
			Resource:      entry.Resource,
			Outcome:       entry.Outcome,
			Reason:        entry.Reason,
			PolicyRuleRefs: append([]string{}, entry.PolicyRuleRefs...),
			OccurredAt:    entry.OccurredAt,
			CorrelationID: entry.CorrelationID,
		})
	}
	return records
}
