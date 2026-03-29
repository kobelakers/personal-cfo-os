package verification

import (
	"fmt"
	"slices"
	"strings"

	"github.com/kobelakers/personal-cfo-os/internal/analysis"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
)

func RunCashflowGroundingPrecheck(candidate analysis.CashflowStructuredCandidate, metrics analysis.CashflowDeterministicMetrics, selectedEvidenceIDs []observation.EvidenceID) error {
	if metrics.MonthlyInflowCents == 0 && metrics.MonthlyOutflowCents == 0 && metrics.MonthlyNetIncomeCents == 0 {
		return fmt.Errorf("cashflow deterministic metrics are empty")
	}
	if len(selectedEvidenceIDs) == 0 {
		return fmt.Errorf("cashflow grounding requires selected evidence")
	}
	allowed := make([]string, 0, len(selectedEvidenceIDs))
	for _, id := range selectedEvidenceIDs {
		allowed = append(allowed, string(id))
	}
	for _, ref := range candidate.EvidenceRefs {
		if !slices.Contains(allowed, ref) {
			return fmt.Errorf("cashflow evidence ref %s is outside selected evidence", ref)
		}
	}
	if diagnostics := cashflowNumericGroundingDiagnostics(candidate, metrics); len(diagnostics) > 0 {
		return fmt.Errorf("%s", strings.Join(diagnostics, "; "))
	}
	return nil
}
