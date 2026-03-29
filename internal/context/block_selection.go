package context

import (
	"strings"

	"github.com/kobelakers/personal-cfo-os/internal/memory"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
)

type BlockContextSpec struct {
	PlanID               string              `json:"plan_id"`
	BlockID              string              `json:"block_id"`
	BlockKind            string              `json:"block_kind"`
	AssignedRecipient    string              `json:"assigned_recipient"`
	Goal                 string              `json:"goal"`
	RequiredEvidenceRefs []string            `json:"required_evidence_refs,omitempty"`
	RequiredMemoryKinds  []memory.MemoryKind `json:"required_memory_kinds,omitempty"`
	RequiredStateBlocks  []string            `json:"required_state_blocks,omitempty"`
	ExecutionView        ContextView         `json:"execution_view"`
	VerificationRules    []string            `json:"verification_rules,omitempty"`
}

func filterStateBlocksByNames(blocks []InjectedStateBlock, names []string) []InjectedStateBlock {
	if len(names) == 0 {
		return blocks
	}
	allowed := make(map[string]struct{}, len(names))
	for _, name := range names {
		allowed[name] = struct{}{}
	}
	filtered := make([]InjectedStateBlock, 0, len(blocks))
	for _, block := range blocks {
		if _, ok := allowed[block.Name]; ok {
			filtered = append(filtered, block)
		}
	}
	return filtered
}

func filterEvidenceBlocksByTypes(blocks []EvidenceSummaryBlock, allowedTypes []string) []EvidenceSummaryBlock {
	if len(allowedTypes) == 0 {
		return blocks
	}
	allowed := make(map[string]struct{}, len(allowedTypes))
	for _, item := range allowedTypes {
		allowed[item] = struct{}{}
	}
	filtered := make([]EvidenceSummaryBlock, 0, len(blocks))
	for _, block := range blocks {
		if _, ok := allowed[string(block.Type)]; ok {
			filtered = append(filtered, block)
		}
	}
	return filtered
}

func filterEvidenceRecordsByTypes(records []observation.EvidenceRecord, allowedTypes []string) []observation.EvidenceRecord {
	if len(allowedTypes) == 0 {
		return records
	}
	allowed := make(map[string]struct{}, len(allowedTypes))
	for _, item := range allowedTypes {
		allowed[item] = struct{}{}
	}
	filtered := make([]observation.EvidenceRecord, 0, len(records))
	for _, record := range records {
		if _, ok := allowed[string(record.Type)]; ok {
			filtered = append(filtered, record)
		}
	}
	return filtered
}

func filterMemoryBlocksForSpec(blocks []MemoryBlock, spec BlockContextSpec) []MemoryBlock {
	filtered := make([]MemoryBlock, 0, len(blocks))
	for _, block := range blocks {
		if matchesMemoryKind(block.Kind, spec.RequiredMemoryKinds) && matchesMemorySummary(block.Summary, spec.BlockKind) {
			filtered = append(filtered, block)
		}
	}
	return filtered
}

func filterMemoryRecordsForSpec(records []memory.MemoryRecord, spec BlockContextSpec) []memory.MemoryRecord {
	filtered := make([]memory.MemoryRecord, 0, len(records))
	for _, record := range records {
		if matchesMemoryKind(record.Kind, spec.RequiredMemoryKinds) && matchesMemorySummary(record.Summary, spec.BlockKind) {
			filtered = append(filtered, record)
		}
	}
	return filtered
}

func matchesMemoryKind(kind memory.MemoryKind, allowed []memory.MemoryKind) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, item := range allowed {
		if item == kind {
			return true
		}
	}
	return false
}

func matchesMemorySummary(summary string, blockKind string) bool {
	summary = strings.ToLower(summary)
	switch {
	case strings.HasPrefix(blockKind, "cashflow_") || strings.Contains(blockKind, "cashflow"):
		return strings.Contains(summary, "subscription") ||
			strings.Contains(summary, "late-night") ||
			strings.Contains(summary, "monthly review") ||
			strings.Contains(summary, "decision") ||
			strings.Contains(summary, "life event")
	case strings.HasPrefix(blockKind, "debt_") || strings.Contains(blockKind, "debt"):
		return strings.Contains(summary, "debt pressure") ||
			strings.Contains(summary, "debt-versus-invest") ||
			strings.Contains(summary, "debt burden") ||
			strings.Contains(summary, "decision") ||
			strings.Contains(summary, "monthly review") ||
			strings.Contains(summary, "housing")
	case strings.HasPrefix(blockKind, "tax_") || strings.Contains(blockKind, "tax"):
		return strings.Contains(summary, "tax signal") ||
			strings.Contains(summary, "withholding") ||
			strings.Contains(summary, "family-related tax") ||
			strings.Contains(summary, "deadline") ||
			strings.Contains(summary, "life event")
	case strings.HasPrefix(blockKind, "portfolio_") || strings.Contains(blockKind, "portfolio"):
		return strings.Contains(summary, "decision") ||
			strings.Contains(summary, "liquidity") ||
			strings.Contains(summary, "life event") ||
			strings.Contains(summary, "debt pressure")
	default:
		return true
	}
}
