package memory

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

type WorkflowMemoryResult struct {
	GeneratedIDs     []string       `json:"generated_ids,omitempty"`
	GeneratedRecords []MemoryRecord `json:"generated_records,omitempty"`
	Retrieved        []MemoryRecord `json:"retrieved,omitempty"`
}

type WorkflowMemoryService struct {
	Writer    MemoryWriter
	Gate      MemoryWriteGate
	Retriever HybridRetriever
	Now       func() time.Time
}

func (s WorkflowMemoryService) SyncMonthlyReview(
	ctx context.Context,
	spec taskspec.TaskSpec,
	workflowID string,
	current state.FinancialWorldState,
	evidence []observation.EvidenceRecord,
) (WorkflowMemoryResult, error) {
	now := s.now()
	records := deriveMonthlyReviewMemories(spec, workflowID, current, evidence, now)
	generatedIDs, err := s.writeRecords(ctx, records)
	if err != nil {
		return WorkflowMemoryResult{}, err
	}
	retrieved, err := s.retrieve(ctx, RetrievalQuery{
		Text:         spec.Goal,
		LexicalTerms: spec.Scope.Areas,
		SemanticHint: "monthly financial review, risk signals, optimization suggestions",
		TopK:         4,
	})
	if err != nil {
		return WorkflowMemoryResult{}, err
	}
	return WorkflowMemoryResult{
		GeneratedIDs:     generatedIDs,
		GeneratedRecords: records,
		Retrieved:        retrieved,
	}, nil
}

func (s WorkflowMemoryService) SyncDebtDecision(
	ctx context.Context,
	spec taskspec.TaskSpec,
	workflowID string,
	current state.FinancialWorldState,
	evidence []observation.EvidenceRecord,
	conclusion string,
) (WorkflowMemoryResult, error) {
	now := s.now()
	records := deriveDebtDecisionMemories(spec, workflowID, current, evidence, conclusion, now)
	generatedIDs, err := s.writeRecords(ctx, records)
	if err != nil {
		return WorkflowMemoryResult{}, err
	}
	retrieved, err := s.retrieve(ctx, RetrievalQuery{
		Text:         spec.Goal,
		LexicalTerms: spec.Scope.Areas,
		SemanticHint: "debt paydown versus investing decision, liquidity, risk tradeoffs",
		TopK:         4,
	})
	if err != nil {
		return WorkflowMemoryResult{}, err
	}
	return WorkflowMemoryResult{
		GeneratedIDs:     generatedIDs,
		GeneratedRecords: records,
		Retrieved:        retrieved,
	}, nil
}

func (s WorkflowMemoryService) retrieve(ctx context.Context, query RetrievalQuery) ([]MemoryRecord, error) {
	if s.Retriever == nil {
		return nil, nil
	}
	results, err := s.Retriever.Retrieve(ctx, query)
	if err != nil {
		return nil, err
	}
	memories := make([]MemoryRecord, 0, len(results))
	for _, result := range results {
		memories = append(memories, result.Memory)
	}
	return memories, nil
}

func (s WorkflowMemoryService) writeRecords(ctx context.Context, records []MemoryRecord) ([]string, error) {
	if s.Writer == nil {
		return nil, nil
	}
	ids := make([]string, 0, len(records))
	for _, record := range records {
		if s.Gate != nil {
			if err := s.Gate.AllowWrite(ctx, record); err != nil {
				return nil, fmt.Errorf("memory write denied for %s: %w", record.ID, err)
			}
		}
		if err := s.Writer.Write(ctx, record); err != nil {
			return nil, err
		}
		ids = append(ids, record.ID)
	}
	return ids, nil
}

func (s WorkflowMemoryService) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func deriveMonthlyReviewMemories(
	spec taskspec.TaskSpec,
	workflowID string,
	current state.FinancialWorldState,
	evidence []observation.EvidenceRecord,
	now time.Time,
) []MemoryRecord {
	records := make([]MemoryRecord, 0, 5)
	if current.BehaviorState.DuplicateSubscriptionCount > 0 {
		records = append(records, MemoryRecord{
			ID:      workflowID + "-memory-subscriptions",
			Kind:    MemoryKindSemantic,
			Summary: "User has recurring subscriptions that should be reviewed during monthly review.",
			Facts: []MemoryFact{
				{Key: "duplicate_subscription_count", Value: fmt.Sprintf("%d", current.BehaviorState.DuplicateSubscriptionCount)},
			},
			Source: MemorySource{
				EvidenceIDs: evidenceIDsByPredicate(evidence, "duplicate_subscription_count"),
				TaskID:      spec.ID,
				WorkflowID:  workflowID,
				Actor:       "memory_steward",
			},
			Confidence: MemoryConfidence{Score: 0.88, Rationale: "derived from recurring subscription evidence"},
			CreatedAt:  now,
			UpdatedAt:  now,
		})
	}
	if current.BehaviorState.LateNightSpendingFrequency > 0 {
		records = append(records, MemoryRecord{
			ID:      workflowID + "-memory-late-night",
			Kind:    MemoryKindEpisodic,
			Summary: "A late-night spending pattern was observed in the current review window.",
			Facts: []MemoryFact{
				{Key: "late_night_spending_frequency", Value: fmt.Sprintf("%.4f", current.BehaviorState.LateNightSpendingFrequency)},
			},
			Source: MemorySource{
				EvidenceIDs: evidenceIDsByType(evidence, observation.EvidenceTypeLateNightSpendingSignal),
				TaskID:      spec.ID,
				WorkflowID:  workflowID,
				Actor:       "memory_steward",
			},
			Confidence: MemoryConfidence{Score: 0.72, Rationale: "derived from late-night spending signal"},
			CreatedAt:  now,
			UpdatedAt:  now,
		})
	}
	if current.LiabilityState.DebtBurdenRatio > 0 {
		records = append(records, MemoryRecord{
			ID:      workflowID + "-memory-debt-pressure",
			Kind:    MemoryKindSemantic,
			Summary: "Monthly review observed current debt pressure and minimum payment load.",
			Facts: []MemoryFact{
				{Key: "debt_burden_ratio", Value: fmt.Sprintf("%.4f", current.LiabilityState.DebtBurdenRatio)},
				{Key: "minimum_payment_pressure", Value: fmt.Sprintf("%.4f", current.LiabilityState.MinimumPaymentPressure)},
			},
			Source: MemorySource{
				EvidenceIDs: evidenceIDsByType(evidence, observation.EvidenceTypeDebtObligationSnapshot),
				TaskID:      spec.ID,
				WorkflowID:  workflowID,
				Actor:       "memory_steward",
			},
			Confidence: MemoryConfidence{Score: 0.9, Rationale: "derived from debt obligation snapshot"},
			CreatedAt:  now,
			UpdatedAt:  now,
		})
	}
	if current.TaxState.ChildcareTaxSignal {
		facts := []MemoryFact{
			{Key: "childcare_tax_signal", Value: "true"},
		}
		if notes := strings.Join(current.TaxState.FamilyTaxNotes, "; "); strings.TrimSpace(notes) != "" {
			facts = append(facts, MemoryFact{Key: "family_tax_notes", Value: notes})
		}
		records = append(records, MemoryRecord{
			ID:      workflowID + "-memory-tax-signal",
			Kind:    MemoryKindSemantic,
			Summary: "Family-related tax optimization signal was present during the review.",
			Facts:   facts,
			Source: MemorySource{
				EvidenceIDs: evidenceIDsByType(evidence, observation.EvidenceTypePayslipStatement, observation.EvidenceTypeTaxDocument),
				TaskID:      spec.ID,
				WorkflowID:  workflowID,
				Actor:       "memory_steward",
			},
			Confidence: MemoryConfidence{Score: 0.84, Rationale: "consistent payroll and tax document signal"},
			CreatedAt:  now,
			UpdatedAt:  now,
		})
	}
	records = append(records, MemoryRecord{
		ID:      workflowID + "-memory-procedure",
		Kind:    MemoryKindProcedural,
		Summary: "Monthly review should always cover cashflow, debt, portfolio, tax, behavior, and risk blocks.",
		Facts: []MemoryFact{
			{Key: "monthly_review_checklist", Value: "cashflow,debt,portfolio,tax,behavior,risk"},
		},
		Source: MemorySource{
			TaskID:     spec.ID,
			WorkflowID: workflowID,
			Actor:      "memory_steward",
		},
		Confidence: MemoryConfidence{Score: 0.95, Rationale: "workflow-generated procedural memory"},
		CreatedAt:  now,
		UpdatedAt:  now,
	})
	return records
}

func deriveDebtDecisionMemories(
	spec taskspec.TaskSpec,
	workflowID string,
	current state.FinancialWorldState,
	evidence []observation.EvidenceRecord,
	conclusion string,
	now time.Time,
) []MemoryRecord {
	records := make([]MemoryRecord, 0, 2)
	if current.LiabilityState.DebtBurdenRatio > 0 || current.LiabilityState.MinimumPaymentPressure > 0 {
		records = append(records, MemoryRecord{
			ID:      workflowID + "-memory-debt-comparison",
			Kind:    MemoryKindSemantic,
			Summary: "Debt-versus-invest decisions should include debt burden, minimum payment pressure, and liquidity coverage.",
			Facts: []MemoryFact{
				{Key: "debt_burden_ratio", Value: fmt.Sprintf("%.4f", current.LiabilityState.DebtBurdenRatio)},
				{Key: "minimum_payment_pressure", Value: fmt.Sprintf("%.4f", current.LiabilityState.MinimumPaymentPressure)},
			},
			Source: MemorySource{
				EvidenceIDs: evidenceIDsByType(evidence, observation.EvidenceTypeDebtObligationSnapshot, observation.EvidenceTypeTransactionBatch),
				TaskID:      spec.ID,
				WorkflowID:  workflowID,
				Actor:       "memory_steward",
			},
			Confidence: MemoryConfidence{Score: 0.87, Rationale: "derived from debt decision baseline evidence"},
			CreatedAt:  now,
			UpdatedAt:  now,
		})
	}
	if strings.TrimSpace(conclusion) != "" {
		records = append(records, MemoryRecord{
			ID:      workflowID + "-memory-latest-decision",
			Kind:    MemoryKindEpisodic,
			Summary: "A debt-versus-invest decision was produced for the current analysis window.",
			Facts: []MemoryFact{
				{Key: "decision_conclusion", Value: conclusion},
			},
			Source: MemorySource{
				EvidenceIDs: evidenceIDsByType(evidence, observation.EvidenceTypeDebtObligationSnapshot, observation.EvidenceTypePortfolioAllocationSnap),
				TaskID:      spec.ID,
				WorkflowID:  workflowID,
				Actor:       "memory_steward",
			},
			Confidence: MemoryConfidence{Score: 0.8, Rationale: "workflow conclusion backed by current evidence window"},
			CreatedAt:  now,
			UpdatedAt:  now,
		})
	}
	return records
}

func evidenceIDsByPredicate(records []observation.EvidenceRecord, predicate string) []observation.EvidenceID {
	ids := make([]observation.EvidenceID, 0)
	for _, record := range records {
		for _, claim := range record.Claims {
			if claim.Predicate == predicate {
				ids = append(ids, record.ID)
				break
			}
		}
	}
	return ids
}

func evidenceIDsByType(records []observation.EvidenceRecord, types ...observation.EvidenceType) []observation.EvidenceID {
	allowed := make(map[observation.EvidenceType]struct{}, len(types))
	for _, evidenceType := range types {
		allowed[evidenceType] = struct{}{}
	}
	ids := make([]observation.EvidenceID, 0)
	for _, record := range records {
		if _, ok := allowed[record.Type]; ok {
			ids = append(ids, record.ID)
		}
	}
	return ids
}
