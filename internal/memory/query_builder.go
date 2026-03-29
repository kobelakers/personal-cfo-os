package memory

import (
	"fmt"
	"strings"

	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

const (
	MemoryConsumerPlanner  = "planner_agent"
	MemoryConsumerCashflow = "cashflow_agent"
)

type QueryBuildInput struct {
	WorkflowID string
	Task       taskspec.TaskSpec
	State      state.FinancialWorldState
	Evidence   []observation.EvidenceRecord
	TraceID    string
}

type MemoryQueryBuilder interface {
	Build(input QueryBuildInput) RetrievalQuery
}

type PlannerMemoryQueryBuilder struct {
	TopK            int
	RetrievalPolicy string
	FreshnessPolicy string
}

func (b PlannerMemoryQueryBuilder) Build(input QueryBuildInput) RetrievalQuery {
	topK := b.TopK
	if topK == 0 {
		topK = 4
	}
	lexicalTerms := dedupeTokens(append([]string{}, input.Task.Scope.Areas...))
	if input.State.BehaviorState.DuplicateSubscriptionCount > 0 {
		lexicalTerms = append(lexicalTerms, "subscriptions", "cashflow")
	}
	if input.State.LiabilityState.DebtBurdenRatio > 0 {
		lexicalTerms = append(lexicalTerms, "debt", "liquidity")
	}
	return RetrievalQuery{
		QueryID:         fmt.Sprintf("%s:%s:planning", input.WorkflowID, input.Task.ID),
		WorkflowID:      input.WorkflowID,
		TaskID:          input.Task.ID,
		TraceID:         input.TraceID,
		Consumer:        MemoryConsumerPlanner,
		ContextView:     "planning",
		Text:            input.Task.Goal,
		LexicalTerms:    dedupeTokens(lexicalTerms),
		SemanticHint:    "monthly financial review planning, prior risk signals, prior recommendations, decision emphasis",
		TopK:            topK,
		RetrievalPolicy: fallbackQueryString(b.RetrievalPolicy, "monthly_review_planning"),
		FreshnessPolicy: fallbackQueryString(b.FreshnessPolicy, "monthly_review_default"),
	}
}

type CashflowMemoryQueryBuilder struct {
	TopK            int
	RetrievalPolicy string
	FreshnessPolicy string
}

func (b CashflowMemoryQueryBuilder) Build(input QueryBuildInput) RetrievalQuery {
	topK := b.TopK
	if topK == 0 {
		topK = 4
	}
	lexicalTerms := append([]string{}, input.Task.Scope.Areas...)
	lexicalTerms = append(lexicalTerms, evidenceKeywords(input.Evidence)...)
	if input.State.BehaviorState.DuplicateSubscriptionCount > 0 {
		lexicalTerms = append(lexicalTerms, "subscription")
	}
	if input.State.BehaviorState.LateNightSpendingFrequency > 0 {
		lexicalTerms = append(lexicalTerms, "late-night", "spending")
	}
	if input.State.CashflowState.MonthlyNetIncomeCents < 0 {
		lexicalTerms = append(lexicalTerms, "net", "cashflow_pressure")
	}
	return RetrievalQuery{
		QueryID:         fmt.Sprintf("%s:%s:cashflow", input.WorkflowID, input.Task.ID),
		WorkflowID:      input.WorkflowID,
		TaskID:          input.Task.ID,
		TraceID:         input.TraceID,
		Consumer:        MemoryConsumerCashflow,
		ContextView:     "execution",
		Text:            input.Task.Goal,
		LexicalTerms:    dedupeTokens(lexicalTerms),
		SemanticHint:    "monthly cashflow review, spending anomalies, recurring subscriptions, cashflow stability, recommendation framing",
		TopK:            topK,
		RetrievalPolicy: fallbackQueryString(b.RetrievalPolicy, "monthly_review_cashflow"),
		FreshnessPolicy: fallbackQueryString(b.FreshnessPolicy, "monthly_review_default"),
	}
}

func evidenceKeywords(evidence []observation.EvidenceRecord) []string {
	keywords := make([]string, 0, len(evidence)*2)
	for _, item := range evidence {
		switch item.Type {
		case observation.EvidenceTypeRecurringSubscription:
			keywords = append(keywords, "subscriptions", "recurring")
		case observation.EvidenceTypeLateNightSpendingSignal:
			keywords = append(keywords, "late-night", "spending")
		case observation.EvidenceTypeDebtObligationSnapshot:
			keywords = append(keywords, "debt", "payment")
		case observation.EvidenceTypeTaxDocument:
			keywords = append(keywords, "tax", "deadline")
		}
	}
	return dedupeTokens(keywords)
}

func fallbackQueryString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
