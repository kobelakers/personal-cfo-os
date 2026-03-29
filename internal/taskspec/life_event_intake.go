package taskspec

import (
	"fmt"
	"strings"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/observation"
)

type EventTriggeredIntakeService struct {
	Now func() time.Time
}

func (s EventTriggeredIntakeService) Build(event observation.LifeEventRecord) TaskIntakeResult {
	if err := event.Validate(); err != nil {
		return TaskIntakeResult{
			Accepted:      false,
			RawInput:      event.KindSummary(),
			Confidence:    TaskIntakeConfidenceLow,
			FailureReason: TaskIntakeFailureValidation,
			Notes:         []string{err.Error()},
		}
	}

	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now().UTC()
	}
	spec := buildLifeEventTaskSpec(event, now)
	if err := spec.Validate(); err != nil {
		return TaskIntakeResult{
			Accepted:      false,
			RawInput:      event.KindSummary(),
			Confidence:    TaskIntakeConfidenceMedium,
			FailureReason: TaskIntakeFailureValidation,
			Notes:         []string{err.Error()},
		}
	}
	return TaskIntakeResult{
		Accepted:   true,
		RawInput:   event.KindSummary(),
		Confidence: TaskIntakeConfidenceHigh,
		Notes: []string{
			"life event trigger normalized into workflow task spec",
			"workflow c constraints and evidence requirements injected",
		},
		TaskSpec: &spec,
	}
}

func buildLifeEventTaskSpec(event observation.LifeEventRecord, now time.Time) TaskSpec {
	scopeAreas := []string{"cashflow", "risk"}
	requiredEvidence := []RequiredEvidenceRef{
		{Type: string(observation.EvidenceTypeEventSignal), Reason: "life event trigger must be grounded in typed event evidence", Mandatory: true},
		{Type: string(observation.EvidenceTypeCalendarDeadline), Reason: "follow-up due windows should use typed deadline evidence when available", Mandatory: false},
	}
	riskLevel := RiskLevelMedium
	approval := ApprovalRequirementRecommended
	goal := fmt.Sprintf("Assess the financial impact of life event %q and generate follow-up finance tasks.", event.Kind)
	notes := []string{"workflow_c", string(event.Kind)}

	switch event.Kind {
	case observation.LifeEventSalaryChange:
		scopeAreas = append(scopeAreas, "tax", "portfolio")
		requiredEvidence = append(requiredEvidence,
			RequiredEvidenceRef{Type: string(observation.EvidenceTypeTransactionBatch), Reason: "cashflow baseline before salary change", Mandatory: true},
			RequiredEvidenceRef{Type: string(observation.EvidenceTypePayslipStatement), Reason: "salary and withholding change confirmation", Mandatory: false},
			RequiredEvidenceRef{Type: string(observation.EvidenceTypePortfolioAllocationSnap), Reason: "portfolio reallocation implications after salary change", Mandatory: false},
		)
	case observation.LifeEventNewChild:
		scopeAreas = append(scopeAreas, "tax")
		requiredEvidence = append(requiredEvidence,
			RequiredEvidenceRef{Type: string(observation.EvidenceTypeTransactionBatch), Reason: "cashflow baseline after family size change", Mandatory: true},
			RequiredEvidenceRef{Type: string(observation.EvidenceTypeTaxDocument), Reason: "family and childcare tax follow-up", Mandatory: false},
			RequiredEvidenceRef{Type: string(observation.EvidenceTypePayslipStatement), Reason: "payroll withholding and dependent changes", Mandatory: false},
		)
	case observation.LifeEventJobChange:
		scopeAreas = append(scopeAreas, "tax", "portfolio")
		requiredEvidence = append(requiredEvidence,
			RequiredEvidenceRef{Type: string(observation.EvidenceTypeTransactionBatch), Reason: "cashflow and liquidity baseline after job change", Mandatory: true},
			RequiredEvidenceRef{Type: string(observation.EvidenceTypePayslipStatement), Reason: "new compensation and withholding baseline", Mandatory: false},
			RequiredEvidenceRef{Type: string(observation.EvidenceTypePortfolioAllocationSnap), Reason: "portfolio contribution and allocation implications", Mandatory: false},
		)
		riskLevel = RiskLevelHigh
		approval = ApprovalRequirementMandatory
	case observation.LifeEventHousingChange:
		scopeAreas = append(scopeAreas, "debt", "portfolio")
		requiredEvidence = append(requiredEvidence,
			RequiredEvidenceRef{Type: string(observation.EvidenceTypeTransactionBatch), Reason: "cashflow baseline after housing cost change", Mandatory: true},
			RequiredEvidenceRef{Type: string(observation.EvidenceTypeDebtObligationSnapshot), Reason: "mortgage or housing debt implications", Mandatory: false},
			RequiredEvidenceRef{Type: string(observation.EvidenceTypePortfolioAllocationSnap), Reason: "liquidity and asset allocation implications", Mandatory: false},
		)
		riskLevel = RiskLevelHigh
		approval = ApprovalRequirementMandatory
	}

	scopeStart, scopeEnd := buildLifeEventAnalysisWindow(event, now)
	deadline := now.Add(72 * time.Hour)
	return TaskSpec{
		ID:    "task-life-event-" + strings.ToLower(string(event.Kind)) + "-" + now.Format("20060102150405"),
		Goal:  goal,
		Scope: TaskScope{Areas: scopeAreas, Start: scopeStart, End: scopeEnd, Notes: notes},
		Constraints: ConstraintSet{
			Hard: []string{
				"life event input must first become typed evidence before workflow execution",
				"generated follow-up tasks must remain convertible to standard TaskSpec",
				"financial metrics must remain deterministic",
			},
			Soft: []string{
				"prefer generated tasks that are grounded in event evidence, state diff, and retrieved memories",
			},
		},
		RiskLevel: riskLevel,
		SuccessCriteria: []SuccessCriteria{
			{ID: "event-grounding", Description: "life event is represented by typed evidence and supporting state diff"},
			{ID: "follow-up-graph", Description: "generated follow-up tasks are typed, grounded, and registered in runtime"},
			{ID: "governed-output", Description: "generated tasks and assessment remain verifiable and governed"},
		},
		RequiredEvidence:    requiredEvidence,
		ApprovalRequirement: approval,
		Deadline:            &deadline,
		UserIntentType:      UserIntentLifeEventTrigger,
		CreatedAt:           now,
	}
}

func buildLifeEventAnalysisWindow(event observation.LifeEventRecord, now time.Time) (*time.Time, *time.Time) {
	start := event.WindowStart()
	end := event.WindowEnd()

	analysisStart := now.AddDate(0, 0, -30).UTC()
	if start != nil {
		candidate := start.AddDate(0, 0, -30).UTC()
		if candidate.Before(analysisStart) {
			analysisStart = candidate
		}
	}

	analysisEnd := now.UTC()
	if end != nil && end.After(analysisEnd) {
		analysisEnd = end.UTC()
	}

	return &analysisStart, &analysisEnd
}
