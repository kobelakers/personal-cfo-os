package memory

import "github.com/kobelakers/personal-cfo-os/internal/observability"

func ToObservabilityRecords(entries []MemoryAccessAudit) []observability.MemoryAccessRecord {
	records := make([]observability.MemoryAccessRecord, 0, len(entries))
	for _, entry := range entries {
		records = append(records, observability.MemoryAccessRecord{
			MemoryID:   entry.MemoryID,
			Accessor:   entry.Accessor,
			Purpose:    entry.Purpose,
			Action:     entry.Action,
			AccessedAt: entry.AccessedAt,
		})
	}
	return records
}
