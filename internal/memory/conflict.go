package memory

import "strings"

// DefaultConflictDetector keeps conflict detection explicit and explainable.
// It models only the narrow 5C rule we can defend today: same fact key with
// different value for the same memory kind means the candidate conflicts.
type DefaultConflictDetector struct{}

func (DefaultConflictDetector) Detect(existing []MemoryRecord, candidate MemoryRecord) []ConflictRef {
	conflicts := make([]ConflictRef, 0)
	for _, record := range existing {
		if record.ID == candidate.ID || record.Kind != candidate.Kind {
			continue
		}
		for _, fact := range candidate.Facts {
			for _, existingFact := range record.Facts {
				if fact.Key == existingFact.Key && fact.Value != existingFact.Value {
					conflicts = append(conflicts, ConflictRef{
						MemoryID: record.ID,
						Reason:   "same fact key with different value",
					})
					goto nextRecord
				}
			}
		}
	nextRecord:
	}
	return dedupeConflictRefs(conflicts)
}

// DefaultSupersedenceDetector keeps memory evolution rules explicit without
// overreaching into a full knowledge-graph reasoning system.
type DefaultSupersedenceDetector struct{}

func (DefaultSupersedenceDetector) Detect(existing []MemoryRecord, candidate MemoryRecord) []SupersedesRef {
	supersedes := make([]SupersedesRef, 0)
	for _, record := range existing {
		if record.ID == candidate.ID || record.Kind != candidate.Kind {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(record.Summary), strings.TrimSpace(candidate.Summary)) && candidate.UpdatedAt.After(record.UpdatedAt) {
			supersedes = append(supersedes, SupersedesRef{
				MemoryID: record.ID,
				Reason:   "same summary updated with newer evidence",
			})
		}
	}
	return dedupeSupersedesRefs(supersedes)
}

func dedupeConflictRefs(items []ConflictRef) []ConflictRef {
	seen := make(map[string]struct{}, len(items))
	result := make([]ConflictRef, 0, len(items))
	for _, item := range items {
		if item.MemoryID == "" {
			continue
		}
		key := item.MemoryID + "::" + item.Reason
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, item)
	}
	return result
}

func dedupeSupersedesRefs(items []SupersedesRef) []SupersedesRef {
	seen := make(map[string]struct{}, len(items))
	result := make([]SupersedesRef, 0, len(items))
	for _, item := range items {
		if item.MemoryID == "" {
			continue
		}
		key := item.MemoryID + "::" + item.Reason
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, item)
	}
	return result
}
