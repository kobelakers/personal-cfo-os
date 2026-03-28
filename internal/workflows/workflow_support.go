package workflows

import (
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

func observationInput(spec taskspec.TaskSpec, userID string) map[string]string {
	input := map[string]string{
		"task_id": spec.ID,
		"user_id": userID,
	}
	if spec.Scope.Start != nil {
		input["start"] = spec.Scope.Start.UTC().Format(time.RFC3339)
	}
	if spec.Scope.End != nil {
		input["end"] = spec.Scope.End.UTC().Format(time.RFC3339)
	}
	return input
}

func dedupeEvidence(records []observation.EvidenceRecord) []observation.EvidenceRecord {
	seen := make(map[observation.EvidenceID]struct{}, len(records))
	result := make([]observation.EvidenceRecord, 0, len(records))
	for _, record := range records {
		if _, ok := seen[record.ID]; ok {
			continue
		}
		seen[record.ID] = struct{}{}
		result = append(result, record)
	}
	return result
}

func collectEvidenceIDs(records []observation.EvidenceRecord) []observation.EvidenceID {
	result := make([]observation.EvidenceID, 0, len(records))
	for _, record := range records {
		result = append(result, record.ID)
	}
	return result
}
