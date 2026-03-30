package verification

import (
	"fmt"
	"slices"
	"strings"

	"github.com/kobelakers/personal-cfo-os/internal/analysis"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/planning"
)

func ValidatePlannerStructuredCandidate(candidate planning.PlannerStructuredCandidate, allowedBlockIDs []string, allowedStepIDs []string) []string {
	schema := planning.PlannerStructuredSchema{
		AllowedBlockIDs: allowedBlockIDs,
		AllowedStepIDs:  allowedStepIDs,
	}
	return schema.Validate(candidate)
}

func ValidateCashflowStructuredCandidate(candidate analysis.CashflowStructuredCandidate, allowedMetricRefs []string, selectedEvidenceIDs []observation.EvidenceID, metrics analysis.CashflowDeterministicMetrics) []string {
	diagnostics := make([]string, 0)
	if strings.TrimSpace(candidate.Summary) == "" {
		diagnostics = append(diagnostics, "summary is required")
	}
	if len(candidate.KeyFindings) == 0 {
		diagnostics = append(diagnostics, "key_findings are required")
	}
	if len(candidate.EvidenceRefs) == 0 {
		diagnostics = append(diagnostics, "evidence_refs are required")
	}
	if candidate.Confidence < 0 || candidate.Confidence > 1 {
		diagnostics = append(diagnostics, "confidence must be within [0,1]")
	}
	if len(candidate.Caveats) == 0 {
		diagnostics = append(diagnostics, "caveats are required")
	}
	for _, ref := range candidate.MetricRefs {
		if !slices.Contains(allowedMetricRefs, ref) {
			diagnostics = append(diagnostics, fmt.Sprintf("unknown metric_ref %s", ref))
		}
	}
	allowedEvidence := make(map[string]struct{}, len(selectedEvidenceIDs))
	for _, id := range selectedEvidenceIDs {
		allowedEvidence[string(id)] = struct{}{}
	}
	for _, ref := range candidate.EvidenceRefs {
		if _, ok := allowedEvidence[ref]; !ok {
			diagnostics = append(diagnostics, fmt.Sprintf("evidence_ref %s is not selected", ref))
		}
	}
	for _, item := range candidate.GroundedRecommendations {
		if strings.TrimSpace(item.Title) == "" || strings.TrimSpace(item.Detail) == "" {
			diagnostics = append(diagnostics, "recommendation title/detail is required")
		}
		if item.Type == "" {
			diagnostics = append(diagnostics, "recommendation type is required")
		}
		if item.RiskLevel == "" {
			diagnostics = append(diagnostics, "recommendation risk_level is required")
		}
		for _, ref := range item.EvidenceRefs {
			if _, ok := allowedEvidence[ref]; !ok {
				diagnostics = append(diagnostics, fmt.Sprintf("recommendation evidence_ref %s is not selected", ref))
			}
		}
	}
	for _, flag := range candidate.RiskFlags {
		if err := validateRiskFlag(flag, allowedEvidence); err != nil {
			diagnostics = append(diagnostics, err.Error())
		}
	}
	diagnostics = append(diagnostics, cashflowNumericGroundingDiagnostics(candidate, metrics)...)
	return diagnostics
}

func validateRiskFlag(flag analysis.RiskFlag, allowedEvidence map[string]struct{}) error {
	if strings.TrimSpace(flag.Code) == "" {
		return fmt.Errorf("risk_flag code is required")
	}
	if strings.TrimSpace(flag.Severity) == "" {
		return fmt.Errorf("risk_flag severity is required")
	}
	if strings.TrimSpace(flag.Detail) == "" {
		return fmt.Errorf("risk_flag detail is required")
	}
	for _, id := range flag.EvidenceIDs {
		if _, ok := allowedEvidence[string(id)]; !ok {
			return fmt.Errorf("risk_flag evidence_ref %s is not selected", id)
		}
	}
	return nil
}
