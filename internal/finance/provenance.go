package finance

import (
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/observation"
)

func intMetric(ref, domain, name string, value int64, unit string, asOf time.Time, evidence []observation.EvidenceRecord, derivation string) MetricRecord {
	return MetricRecord{
		Ref:          ref,
		Domain:       domain,
		Name:         name,
		ValueType:    MetricValueTypeInt64,
		Int64Value:   value,
		Unit:         unit,
		AsOf:         asOf.UTC(),
		SourceRefs:   sourceRefs(evidence),
		EvidenceRefs: evidenceRefs(evidence),
		Derivation:   derivation,
	}
}

func floatMetric(ref, domain, name string, value float64, unit string, asOf time.Time, evidence []observation.EvidenceRecord, derivation string) MetricRecord {
	return MetricRecord{
		Ref:          ref,
		Domain:       domain,
		Name:         name,
		ValueType:    MetricValueTypeFloat64,
		Float64Value: value,
		Unit:         unit,
		AsOf:         asOf.UTC(),
		SourceRefs:   sourceRefs(evidence),
		EvidenceRefs: evidenceRefs(evidence),
		Derivation:   derivation,
	}
}

func boolMetric(ref, domain, name string, value bool, asOf time.Time, evidence []observation.EvidenceRecord, derivation string) MetricRecord {
	return MetricRecord{
		Ref:          ref,
		Domain:       domain,
		Name:         name,
		ValueType:    MetricValueTypeBool,
		BoolValue:    value,
		AsOf:         asOf.UTC(),
		SourceRefs:   sourceRefs(evidence),
		EvidenceRefs: evidenceRefs(evidence),
		Derivation:   derivation,
	}
}

func stringMetric(ref, domain, name, value string, asOf time.Time, evidence []observation.EvidenceRecord, derivation string) MetricRecord {
	return MetricRecord{
		Ref:          ref,
		Domain:       domain,
		Name:         name,
		ValueType:    MetricValueTypeString,
		StringValue:  value,
		AsOf:         asOf.UTC(),
		SourceRefs:   sourceRefs(evidence),
		EvidenceRefs: evidenceRefs(evidence),
		Derivation:   derivation,
	}
}

func sourceRefs(evidence []observation.EvidenceRecord) []string {
	result := make([]string, 0, len(evidence))
	for _, item := range evidence {
		result = append(result, item.Source.Reference)
	}
	return uniqueStrings(result)
}

func evidenceRefs(evidence []observation.EvidenceRecord) []string {
	result := make([]string, 0, len(evidence))
	for _, item := range evidence {
		result = append(result, string(item.ID))
	}
	return uniqueStrings(result)
}

func uniqueStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}
