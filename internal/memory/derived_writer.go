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
	PlanningRetrieved []MemoryRecord `json:"planning_retrieved,omitempty"`
	CashflowRetrieved []MemoryRecord `json:"cashflow_retrieved,omitempty"`
}

type WorkflowMemoryService struct {
	Writer                MemoryWriter
	Gate                  MemoryWriteGate
	Retriever             HybridRetriever
	PlannerQueryBuilder   MemoryQueryBuilder
	CashflowQueryBuilder  MemoryQueryBuilder
	TraceRecorder         MemoryTraceRecorder
	Now                   func() time.Time
}

func (s WorkflowMemoryService) SyncMonthlyReview(
	ctx context.Context,
	spec taskspec.TaskSpec,
	workflowID string,
	current state.FinancialWorldState,
	evidence []observation.EvidenceRecord,
) (WorkflowMemoryResult, error) {
	now := s.now()
	planningQuery, planningRetrieved, planningResults, err := s.retrieveQuery(ctx, s.plannerBuilder(), QueryBuildInput{
		WorkflowID: workflowID,
		Task:       spec,
		State:      current,
		Evidence:   evidence,
		TraceID:    workflowID,
	})
	if err != nil {
		return WorkflowMemoryResult{}, err
	}
	cashflowQuery, cashflowRetrieved, cashflowResults, err := s.retrieveQuery(ctx, s.cashflowBuilder(), QueryBuildInput{
		WorkflowID: workflowID,
		Task:       spec,
		State:      current,
		Evidence:   evidence,
		TraceID:    workflowID,
	})
	if err != nil {
		return WorkflowMemoryResult{}, err
	}
	retrieved := mergeRetrievedMemories(planningRetrieved, cashflowRetrieved)
	s.recordSelection(workflowID, spec.ID, MemoryConsumerPlanner, planningQuery.QueryID, planningResults)
	s.recordSelection(workflowID, spec.ID, MemoryConsumerCashflow, cashflowQuery.QueryID, cashflowResults)
	records := deriveMonthlyReviewMemories(spec, workflowID, current, evidence, now)
	generatedIDs, err := s.writeRecords(ctx, records)
	if err != nil {
		return WorkflowMemoryResult{}, err
	}
	return WorkflowMemoryResult{
		GeneratedIDs:     generatedIDs,
		GeneratedRecords: records,
		Retrieved:        retrieved,
		PlanningRetrieved: planningRetrieved,
		CashflowRetrieved: cashflowRetrieved,
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

func (s WorkflowMemoryService) SyncLifeEvent(
	ctx context.Context,
	spec taskspec.TaskSpec,
	workflowID string,
	current state.FinancialWorldState,
	evidence []observation.EvidenceRecord,
) (WorkflowMemoryResult, error) {
	now := s.now()
	records := deriveLifeEventMemories(spec, workflowID, current, evidence, now)
	generatedIDs, err := s.writeRecords(ctx, records)
	if err != nil {
		return WorkflowMemoryResult{}, err
	}
	retrieved, err := s.retrieve(ctx, RetrievalQuery{
		Text:         spec.Goal,
		LexicalTerms: spec.Scope.Areas,
		SemanticHint: "life event impact, follow-up task generation, deadline awareness, tax signal, debt pressure",
		TopK:         5,
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

func (s WorkflowMemoryService) SyncTaxOptimization(
	ctx context.Context,
	spec taskspec.TaskSpec,
	workflowID string,
	current state.FinancialWorldState,
	evidence []observation.EvidenceRecord,
) (WorkflowMemoryResult, error) {
	now := s.now()
	records := deriveLifeEventMemories(spec, workflowID, current, evidence, now)
	generatedIDs, err := s.writeRecords(ctx, records)
	if err != nil {
		return WorkflowMemoryResult{}, err
	}
	retrieved, err := s.retrieve(ctx, RetrievalQuery{
		Text:         spec.Goal,
		LexicalTerms: spec.Scope.Areas,
		SemanticHint: "tax optimization, withholding review, tax deadlines, tax-advantaged contribution follow-up",
		TopK:         5,
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

func (s WorkflowMemoryService) SyncPortfolioRebalance(
	ctx context.Context,
	spec taskspec.TaskSpec,
	workflowID string,
	current state.FinancialWorldState,
	evidence []observation.EvidenceRecord,
) (WorkflowMemoryResult, error) {
	now := s.now()
	records := deriveLifeEventMemories(spec, workflowID, current, evidence, now)
	generatedIDs, err := s.writeRecords(ctx, records)
	if err != nil {
		return WorkflowMemoryResult{}, err
	}
	retrieved, err := s.retrieve(ctx, RetrievalQuery{
		Text:         spec.Goal,
		LexicalTerms: spec.Scope.Areas,
		SemanticHint: "portfolio rebalance, liquidity buffer, allocation drift, event-driven contribution changes",
		TopK:         5,
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
		if result.Rejected || result.MemoryID == "" {
			continue
		}
		memories = append(memories, result.Memory)
	}
	return memories, nil
}

func (s WorkflowMemoryService) retrieveQuery(ctx context.Context, builder MemoryQueryBuilder, input QueryBuildInput) (RetrievalQuery, []MemoryRecord, []RetrievalResult, error) {
	if s.Retriever == nil || builder == nil {
		return RetrievalQuery{}, nil, nil, nil
	}
	query := builder.Build(input)
	s.recordQuery(query)
	results, err := s.Retriever.Retrieve(ctx, query)
	if err != nil {
		return query, nil, nil, err
	}
	s.recordRetrieval(query, results)
	memories := make([]MemoryRecord, 0, len(results))
	for i := range results {
		if results[i].Rejected || results[i].MemoryID == "" {
			continue
		}
		results[i].Selected = true
		memories = append(memories, results[i].Memory)
	}
	return query, memories, results, nil
}

func (s WorkflowMemoryService) writeRecords(ctx context.Context, records []MemoryRecord) ([]string, error) {
	if s.Writer == nil {
		return nil, nil
	}
	ids := make([]string, 0, len(records))
	for _, record := range records {
		if s.Gate != nil {
			if err := s.Gate.AllowWrite(ctx, record); err != nil {
				return nil, &PolicyDeniedError{Reason: fmt.Sprintf("%s for %s", err.Error(), record.ID)}
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

func (s WorkflowMemoryService) plannerBuilder() MemoryQueryBuilder {
	if s.PlannerQueryBuilder != nil {
		return s.PlannerQueryBuilder
	}
	return PlannerMemoryQueryBuilder{}
}

func (s WorkflowMemoryService) cashflowBuilder() MemoryQueryBuilder {
	if s.CashflowQueryBuilder != nil {
		return s.CashflowQueryBuilder
	}
	return CashflowMemoryQueryBuilder{}
}

func (s WorkflowMemoryService) recordQuery(query RetrievalQuery) {
	if s.TraceRecorder == nil {
		return
	}
	s.TraceRecorder.RecordMemoryQuery(MemoryQueryRecord{
		QueryID:          query.QueryID,
		WorkflowID:       query.WorkflowID,
		TaskID:           query.TaskID,
		TraceID:          query.TraceID,
		Consumer:         query.Consumer,
		ContextView:      query.ContextView,
		Text:             query.Text,
		LexicalTerms:     append([]string{}, query.LexicalTerms...),
		SemanticHint:     query.SemanticHint,
		TopK:             query.TopK,
		RetrievalPolicy:  query.RetrievalPolicy,
		FreshnessPolicy:  query.FreshnessPolicy,
		EmbeddingModel:   query.EmbeddingModel,
		EmbeddingProfile: query.EmbeddingProfile,
		OccurredAt:       s.now(),
	})
}

func (s WorkflowMemoryService) recordRetrieval(query RetrievalQuery, results []RetrievalResult) {
	if s.TraceRecorder == nil {
		return
	}
	selected := make([]string, 0)
	rejected := make([]string, 0)
	for _, result := range results {
		if result.MemoryID == "" {
			continue
		}
		if result.Rejected {
			rejected = append(rejected, result.MemoryID)
			continue
		}
		selected = append(selected, result.MemoryID)
	}
	s.TraceRecorder.RecordMemoryRetrieval(MemoryRetrievalRecord{
		QueryID:          query.QueryID,
		WorkflowID:       query.WorkflowID,
		TaskID:           query.TaskID,
		TraceID:          query.TraceID,
		Consumer:         query.Consumer,
		RetrievalPolicy:  query.RetrievalPolicy,
		FreshnessPolicy:  query.FreshnessPolicy,
		FusionSummary:    "lexical+semantic via reciprocal rank fusion",
		SelectedMemoryID: selected,
		RejectedMemoryID: rejected,
		Results:          append([]RetrievalResult{}, results...),
		OccurredAt:       s.now(),
	})
}

func (s WorkflowMemoryService) recordSelection(workflowID string, taskID string, consumer string, queryID string, results []RetrievalResult) {
	if s.TraceRecorder == nil {
		return
	}
	selected := make([]string, 0)
	rejected := make([]string, 0)
	for _, result := range results {
		if result.MemoryID == "" {
			continue
		}
		if result.Rejected {
			rejected = append(rejected, result.MemoryID)
			continue
		}
		selected = append(selected, result.MemoryID)
	}
	s.TraceRecorder.RecordMemorySelection(MemorySelectionRecord{
		QueryID:           queryID,
		WorkflowID:        workflowID,
		TaskID:            taskID,
		TraceID:           workflowID,
		Consumer:          consumer,
		SelectedMemoryIDs: selected,
		RejectedMemoryIDs: rejected,
		Reason:            "selected memories injected into downstream context",
		OccurredAt:        s.now(),
	})
}

func mergeRetrievedMemories(groups ...[]MemoryRecord) []MemoryRecord {
	seen := make(map[string]struct{})
	result := make([]MemoryRecord, 0)
	for _, group := range groups {
		for _, record := range group {
			if _, ok := seen[record.ID]; ok {
				continue
			}
			seen[record.ID] = struct{}{}
			result = append(result, record)
		}
	}
	return result
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

func deriveLifeEventMemories(
	spec taskspec.TaskSpec,
	workflowID string,
	current state.FinancialWorldState,
	evidence []observation.EvidenceRecord,
	now time.Time,
) []MemoryRecord {
	records := make([]MemoryRecord, 0, 4)
	eventKind := detectLifeEventKind(evidence)
	if eventKind != "" {
		records = append(records, MemoryRecord{
			ID:      workflowID + "-memory-life-event",
			Kind:    MemoryKindEpisodic,
			Summary: fmt.Sprintf("Life event %s was ingested and reduced into state for proactive follow-up generation.", eventKind),
			Facts: []MemoryFact{
				{Key: "life_event_kind", Value: eventKind},
			},
			Source: MemorySource{
				EvidenceIDs: evidenceIDsByType(evidence, observation.EvidenceTypeEventSignal),
				TaskID:      spec.ID,
				WorkflowID:  workflowID,
				Actor:       "memory_steward",
			},
			Confidence: MemoryConfidence{Score: 0.86, Rationale: "derived from normalized life event evidence"},
			CreatedAt:  now,
			UpdatedAt:  now,
		})
	}
	if current.TaxState.ChildcareTaxSignal || len(current.TaxState.UpcomingDeadlines) > 0 || hasEvidencePredicate(evidence, "withholding_review_required") {
		records = append(records, MemoryRecord{
			ID:      workflowID + "-memory-life-event-tax-signal",
			Kind:    MemoryKindSemantic,
			Summary: "Life event introduced tax signal, withholding review, or deadline-sensitive tax follow-up.",
			Facts: []MemoryFact{
				{Key: "childcare_tax_signal", Value: fmt.Sprintf("%t", current.TaxState.ChildcareTaxSignal)},
				{Key: "deadline_count", Value: fmt.Sprintf("%d", len(current.TaxState.UpcomingDeadlines))},
			},
			Source: MemorySource{
				EvidenceIDs: evidenceIDsByType(evidence, observation.EvidenceTypeEventSignal, observation.EvidenceTypeCalendarDeadline, observation.EvidenceTypeTaxDocument, observation.EvidenceTypePayslipStatement),
				TaskID:      spec.ID,
				WorkflowID:  workflowID,
				Actor:       "memory_steward",
			},
			Confidence: MemoryConfidence{Score: 0.88, Rationale: "event evidence and tax/deadline state indicate tax-priority follow-up"},
			CreatedAt:  now,
			UpdatedAt:  now,
		})
	}
	if eventKind == string(observation.LifeEventHousingChange) || current.LiabilityState.DebtBurdenRatio > 0.18 || hasEvidencePredicate(evidence, "mortgage_balance_cents") {
		records = append(records, MemoryRecord{
			ID:      workflowID + "-memory-life-event-debt-pressure",
			Kind:    MemoryKindSemantic,
			Summary: "Life event may elevate debt pressure and housing-related liability review priority.",
			Facts: []MemoryFact{
				{Key: "debt_burden_ratio", Value: fmt.Sprintf("%.4f", current.LiabilityState.DebtBurdenRatio)},
				{Key: "minimum_payment_pressure", Value: fmt.Sprintf("%.4f", current.LiabilityState.MinimumPaymentPressure)},
			},
			Source: MemorySource{
				EvidenceIDs: evidenceIDsByType(evidence, observation.EvidenceTypeEventSignal, observation.EvidenceTypeDebtObligationSnapshot),
				TaskID:      spec.ID,
				WorkflowID:  workflowID,
				Actor:       "memory_steward",
			},
			Confidence: MemoryConfidence{Score: 0.84, Rationale: "housing/debt evidence suggests elevated debt review priority"},
			CreatedAt:  now,
			UpdatedAt:  now,
		})
	}
	records = append(records, MemoryRecord{
		ID:      workflowID + "-memory-life-event-procedure",
		Kind:    MemoryKindProcedural,
		Summary: "Life event workflows should update state, verify generated tasks, and register follow-up TaskSpec objects in runtime.",
		Facts: []MemoryFact{
			{Key: "workflow_c_checklist", Value: "observe,reduce,memory,plan,domain,verify,task_generate,govern,register"},
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

func detectLifeEventKind(records []observation.EvidenceRecord) string {
	for _, record := range records {
		if record.Type != observation.EvidenceTypeEventSignal {
			continue
		}
		for _, claim := range record.Claims {
			if claim.Predicate == "life_event_kind" {
				return strings.Trim(claim.ValueJSON, "\"")
			}
		}
	}
	return ""
}

func hasEvidencePredicate(records []observation.EvidenceRecord, predicate string) bool {
	for _, record := range records {
		for _, claim := range record.Claims {
			if claim.Predicate == predicate {
				return true
			}
		}
	}
	return false
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
