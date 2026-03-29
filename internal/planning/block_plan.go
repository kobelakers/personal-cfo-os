package planning

import (
	"strings"
	"time"

	contextview "github.com/kobelakers/personal-cfo-os/internal/context"
	"github.com/kobelakers/personal-cfo-os/internal/memory"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

const (
	BlockRecipientCashflowAgent  = "cashflow_agent"
	BlockRecipientDebtAgent      = "debt_agent"
	BlockRecipientTaxAgent       = "tax_agent"
	BlockRecipientPortfolioAgent = "portfolio_agent"
)

type ExecutionBlockID string
type ExecutionBlockKind string

const (
	ExecutionBlockKindCashflowReview       ExecutionBlockKind = "cashflow_review_block"
	ExecutionBlockKindDebtReview           ExecutionBlockKind = "debt_review_block"
	ExecutionBlockKindCashflowLiquidity    ExecutionBlockKind = "cashflow_liquidity_block"
	ExecutionBlockKindDebtTradeoff         ExecutionBlockKind = "debt_tradeoff_block"
	ExecutionBlockKindCashflowEventImpact  ExecutionBlockKind = "cashflow_event_impact_block"
	ExecutionBlockKindDebtHousingImpact    ExecutionBlockKind = "debt_housing_impact_block"
	ExecutionBlockKindTaxEventImpact       ExecutionBlockKind = "tax_event_impact_block"
	ExecutionBlockKindPortfolioEventImpact ExecutionBlockKind = "portfolio_event_impact_block"
)

type ExecutionBlockDependency struct {
	BlockID ExecutionBlockID `json:"block_id"`
}

type ExecutionBlockRequirement struct {
	RequirementID string `json:"requirement_id"`
	Type          string `json:"type"`
	Mandatory     bool   `json:"mandatory"`
}

type ExecutionBlockSuccessCriteria struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}

type ExecutionBlockVerificationHint struct {
	Rule        string `json:"rule"`
	Description string `json:"description"`
}

type ExecutionBlockRiskHint struct {
	Level     string `json:"level"`
	Rationale string `json:"rationale"`
}

// ExecutionBlock is the load-bearing execution unit that downstream dispatch,
// reporting, and verification must consume instead of rebuilding intent logic.
type ExecutionBlock struct {
	ID                   ExecutionBlockID                 `json:"id"`
	Kind                 ExecutionBlockKind               `json:"kind"`
	AssignedRecipient    string                           `json:"assigned_recipient"`
	Goal                 string                           `json:"goal"`
	RequiredEvidenceRefs []ExecutionBlockRequirement      `json:"required_evidence_refs"`
	RequiredMemoryKinds  []memory.MemoryKind              `json:"required_memory_kinds,omitempty"`
	RequiredStateBlocks  []string                         `json:"required_state_blocks,omitempty"`
	ExecutionContextView contextview.ContextView          `json:"execution_context_view"`
	SuccessCriteria      []ExecutionBlockSuccessCriteria  `json:"success_criteria"`
	VerificationHints    []ExecutionBlockVerificationHint `json:"verification_hints"`
	RiskHints            []ExecutionBlockRiskHint         `json:"risk_hints,omitempty"`
	DependsOn            []ExecutionBlockDependency       `json:"depends_on,omitempty"`
}

// ExecutionBlockPlan is a replayable snapshot of the block-level plan.
type ExecutionBlockPlan struct {
	PlanID       string           `json:"plan_id"`
	WorkflowID   string           `json:"workflow_id"`
	TaskID       string           `json:"task_id"`
	IntentType   string           `json:"intent_type"`
	CreatedAt    time.Time        `json:"created_at"`
	Blocks       []ExecutionBlock `json:"blocks"`
	PlanningView string           `json:"planning_view,omitempty"`
}

func monthlyReviewBlocks(spec taskspec.TaskSpec, slice contextview.ContextSlice) []ExecutionBlock {
	cashflowBlock := ExecutionBlock{
		ID:                ExecutionBlockID("cashflow-review"),
		Kind:              ExecutionBlockKindCashflowReview,
		AssignedRecipient: BlockRecipientCashflowAgent,
		Goal:              "分析本月现金流、储蓄率、重复订阅与深夜消费信号，并给出 evidence-backed 建议。",
		RequiredEvidenceRefs: requiredByType(spec.RequiredEvidence,
			"transaction_batch",
			"recurring_subscription_signal",
			"late_night_spending_signal",
			"payslip_statement",
		),
		RequiredMemoryKinds:  []memory.MemoryKind{memory.MemoryKindSemantic, memory.MemoryKindEpisodic, memory.MemoryKindProcedural},
		RequiredStateBlocks:  []string{"cashflow_state", "behavior_state", "risk_state"},
		ExecutionContextView: contextview.ContextViewExecution,
		SuccessCriteria: []ExecutionBlockSuccessCriteria{
			{ID: "cashflow-metrics", Description: "monthly inflow, outflow, savings rate, and behavior signals are explicitly surfaced"},
		},
		VerificationHints: []ExecutionBlockVerificationHint{
			{Rule: "cashflow_grounding", Description: "recommendations must reference selected transaction or behavior evidence"},
		},
		RiskHints: []ExecutionBlockRiskHint{
			{Level: "medium", Rationale: "cashflow pressure or spending volatility may materially change the review emphasis"},
		},
	}

	debtBlock := ExecutionBlock{
		ID:                ExecutionBlockID("debt-review"),
		Kind:              ExecutionBlockKindDebtReview,
		AssignedRecipient: BlockRecipientDebtAgent,
		Goal:              "分析债务负担、最低还款压力和高风险债务暴露，并给出债务侧建议。",
		RequiredEvidenceRefs: requiredByType(spec.RequiredEvidence,
			"debt_obligation_snapshot",
			"transaction_batch",
			"credit_card_statement",
		),
		RequiredMemoryKinds:  []memory.MemoryKind{memory.MemoryKindSemantic, memory.MemoryKindProcedural},
		RequiredStateBlocks:  []string{"liability_state", "cashflow_state", "risk_state"},
		ExecutionContextView: contextview.ContextViewExecution,
		SuccessCriteria: []ExecutionBlockSuccessCriteria{
			{ID: "debt-risk-coverage", Description: "debt burden, APR pressure, and minimum payment pressure are reflected in the output"},
		},
		VerificationHints: []ExecutionBlockVerificationHint{
			{Rule: "debt_grounding", Description: "debt recommendations must cite debt snapshot or credit card evidence"},
		},
		RiskHints: []ExecutionBlockRiskHint{
			{Level: "high", Rationale: "high debt pressure should elevate debt analysis priority and downstream governance sensitivity"},
		},
		DependsOn: []ExecutionBlockDependency{{BlockID: cashflowBlock.ID}},
	}

	if hasDebtPriorityMemory(slice) {
		cashflowBlock.DependsOn = []ExecutionBlockDependency{{BlockID: debtBlock.ID}}
		debtBlock.DependsOn = nil
		debtBlock.RiskHints = append(debtBlock.RiskHints, ExecutionBlockRiskHint{
			Level:     "high",
			Rationale: "retrieved debt-pressure memory raised debt block priority for this review window",
		})
		return []ExecutionBlock{debtBlock, cashflowBlock}
	}
	return []ExecutionBlock{cashflowBlock, debtBlock}
}

func debtDecisionBlocks(spec taskspec.TaskSpec, slice contextview.ContextSlice) []ExecutionBlock {
	cashflowBlock := ExecutionBlock{
		ID:                ExecutionBlockID("cashflow-liquidity"),
		Kind:              ExecutionBlockKindCashflowLiquidity,
		AssignedRecipient: BlockRecipientCashflowAgent,
		Goal:              "评估继续投资或提前还贷前的流动性缓冲、现金流净结余和消费波动。",
		RequiredEvidenceRefs: requiredByType(spec.RequiredEvidence,
			"transaction_batch",
			"portfolio_allocation_snapshot",
			"payslip_statement",
		),
		RequiredMemoryKinds:  []memory.MemoryKind{memory.MemoryKindSemantic, memory.MemoryKindEpisodic},
		RequiredStateBlocks:  []string{"cashflow_state", "portfolio_state", "risk_state"},
		ExecutionContextView: contextview.ContextViewExecution,
		SuccessCriteria: []ExecutionBlockSuccessCriteria{
			{ID: "liquidity-coverage", Description: "liquidity and cashflow cushion are made explicit before any decision recommendation"},
		},
		VerificationHints: []ExecutionBlockVerificationHint{
			{Rule: "liquidity_grounding", Description: "cashflow/liquidity recommendation must be grounded in selected transaction or portfolio evidence"},
		},
		RiskHints: []ExecutionBlockRiskHint{
			{Level: "medium", Rationale: "weak liquidity coverage can change whether debt paydown should be prioritized"},
		},
	}

	debtBlock := ExecutionBlock{
		ID:                ExecutionBlockID("debt-tradeoff"),
		Kind:              ExecutionBlockKindDebtTradeoff,
		AssignedRecipient: BlockRecipientDebtAgent,
		Goal:              "比较提前还贷与继续投资的债务压力、APR 暴露与最低还款影响。",
		RequiredEvidenceRefs: requiredByType(spec.RequiredEvidence,
			"debt_obligation_snapshot",
			"transaction_batch",
			"portfolio_allocation_snapshot",
		),
		RequiredMemoryKinds:  []memory.MemoryKind{memory.MemoryKindSemantic, memory.MemoryKindEpisodic},
		RequiredStateBlocks:  []string{"liability_state", "cashflow_state", "risk_state"},
		ExecutionContextView: contextview.ContextViewExecution,
		SuccessCriteria: []ExecutionBlockSuccessCriteria{
			{ID: "tradeoff-grounding", Description: "debt tradeoff recommendation explicitly reflects APR, debt burden, and payment pressure"},
		},
		VerificationHints: []ExecutionBlockVerificationHint{
			{Rule: "decision_grounding", Description: "tradeoff conclusion must cite debt snapshot and relevant cashflow evidence"},
		},
		RiskHints: []ExecutionBlockRiskHint{
			{Level: "high", Rationale: "high APR or minimum payment pressure should bias the decision toward debt risk reduction"},
		},
		DependsOn: []ExecutionBlockDependency{{BlockID: cashflowBlock.ID}},
	}

	if hasDebtPriorityMemory(slice) {
		cashflowBlock.DependsOn = []ExecutionBlockDependency{{BlockID: debtBlock.ID}}
		debtBlock.DependsOn = nil
		debtBlock.RiskHints = append(debtBlock.RiskHints, ExecutionBlockRiskHint{
			Level:     "high",
			Rationale: "retrieved debt decision memory increased debt tradeoff priority",
		})
		return []ExecutionBlock{debtBlock, cashflowBlock}
	}
	return []ExecutionBlock{cashflowBlock, debtBlock}
}

func lifeEventBlocks(spec taskspec.TaskSpec, slice contextview.ContextSlice) []ExecutionBlock {
	switch {
	case taskHasScopeNote(spec, "new_child"):
		return []ExecutionBlock{
			taxEventBlock(spec),
			cashflowEventBlock(spec, []ExecutionBlockDependency{{BlockID: ExecutionBlockID("tax-event-impact")}}),
		}
	case taskHasScopeNote(spec, "housing_change"):
		return []ExecutionBlock{
			debtHousingImpactBlock(spec),
			cashflowEventBlock(spec, []ExecutionBlockDependency{{BlockID: ExecutionBlockID("debt-housing-impact")}}),
			portfolioEventBlock(spec, []ExecutionBlockDependency{{BlockID: ExecutionBlockID("cashflow-event-impact")}}),
		}
	case taskHasScopeNote(spec, "job_change"):
		if hasTaxPriorityMemory(slice) {
			return []ExecutionBlock{
				taxEventBlock(spec),
				cashflowEventBlock(spec, []ExecutionBlockDependency{{BlockID: ExecutionBlockID("tax-event-impact")}}),
				portfolioEventBlock(spec, []ExecutionBlockDependency{{BlockID: ExecutionBlockID("cashflow-event-impact")}}),
			}
		}
		return []ExecutionBlock{
			cashflowEventBlock(spec, nil),
			taxEventBlock(spec),
			portfolioEventBlock(spec, []ExecutionBlockDependency{{BlockID: ExecutionBlockID("tax-event-impact")}}),
		}
	default:
		if hasTaxPriorityMemory(slice) {
			return []ExecutionBlock{
				taxEventBlock(spec),
				cashflowEventBlock(spec, []ExecutionBlockDependency{{BlockID: ExecutionBlockID("tax-event-impact")}}),
				portfolioEventBlock(spec, []ExecutionBlockDependency{{BlockID: ExecutionBlockID("cashflow-event-impact")}}),
			}
		}
		return []ExecutionBlock{
			cashflowEventBlock(spec, nil),
			taxEventBlock(spec),
			portfolioEventBlock(spec, []ExecutionBlockDependency{{BlockID: ExecutionBlockID("tax-event-impact")}}),
		}
	}
}

func cashflowEventBlock(spec taskspec.TaskSpec, dependsOn []ExecutionBlockDependency) ExecutionBlock {
	return ExecutionBlock{
		ID:                ExecutionBlockID("cashflow-event-impact"),
		Kind:              ExecutionBlockKindCashflowEventImpact,
		AssignedRecipient: BlockRecipientCashflowAgent,
		Goal:              "评估 life event 对现金流、月度结余与流动性缓冲的影响。",
		RequiredEvidenceRefs: requiredByType(spec.RequiredEvidence,
			"event_signal",
			"calendar_deadline",
			"transaction_batch",
			"payslip_statement",
		),
		RequiredMemoryKinds:  []memory.MemoryKind{memory.MemoryKindSemantic, memory.MemoryKindEpisodic, memory.MemoryKindProcedural},
		RequiredStateBlocks:  []string{"cashflow_state", "risk_state", "workflow_state"},
		ExecutionContextView: contextview.ContextViewExecution,
		SuccessCriteria: []ExecutionBlockSuccessCriteria{
			{ID: "cashflow-event-impact", Description: "event-driven cashflow delta and liquidity implications are explicit and grounded"},
		},
		VerificationHints: []ExecutionBlockVerificationHint{
			{Rule: "cashflow_event_grounding", Description: "cashflow event recommendations must cite event and supporting transaction evidence"},
		},
		RiskHints: []ExecutionBlockRiskHint{
			{Level: "medium", Rationale: "event-driven cashflow changes can shift downstream tax and portfolio urgency"},
		},
		DependsOn: dependsOn,
	}
}

func debtHousingImpactBlock(spec taskspec.TaskSpec) ExecutionBlock {
	return ExecutionBlock{
		ID:                ExecutionBlockID("debt-housing-impact"),
		Kind:              ExecutionBlockKindDebtHousingImpact,
		AssignedRecipient: BlockRecipientDebtAgent,
		Goal:              "评估 housing change 对债务压力、按揭余额和最低还款覆盖的影响。",
		RequiredEvidenceRefs: requiredByType(spec.RequiredEvidence,
			"event_signal",
			"debt_obligation_snapshot",
			"transaction_batch",
		),
		RequiredMemoryKinds:  []memory.MemoryKind{memory.MemoryKindSemantic, memory.MemoryKindProcedural},
		RequiredStateBlocks:  []string{"liability_state", "cashflow_state", "risk_state"},
		ExecutionContextView: contextview.ContextViewExecution,
		SuccessCriteria: []ExecutionBlockSuccessCriteria{
			{ID: "housing-debt-impact", Description: "housing-event debt pressure and liability impact are explicit and grounded"},
		},
		VerificationHints: []ExecutionBlockVerificationHint{
			{Rule: "housing_debt_grounding", Description: "debt housing recommendations must cite mortgage or debt evidence"},
		},
		RiskHints: []ExecutionBlockRiskHint{
			{Level: "high", Rationale: "housing changes often create immediate debt and liquidity pressure"},
		},
	}
}

func taxEventBlock(spec taskspec.TaskSpec) ExecutionBlock {
	return ExecutionBlock{
		ID:                ExecutionBlockID("tax-event-impact"),
		Kind:              ExecutionBlockKindTaxEventImpact,
		AssignedRecipient: BlockRecipientTaxAgent,
		Goal:              "评估 life event 对税务、预扣和福利截止日期的影响。",
		RequiredEvidenceRefs: requiredByType(spec.RequiredEvidence,
			"event_signal",
			"calendar_deadline",
			"tax_document",
			"payslip_statement",
		),
		RequiredMemoryKinds:  []memory.MemoryKind{memory.MemoryKindSemantic, memory.MemoryKindProcedural},
		RequiredStateBlocks:  []string{"tax_state", "cashflow_state", "risk_state"},
		ExecutionContextView: contextview.ContextViewExecution,
		SuccessCriteria: []ExecutionBlockSuccessCriteria{
			{ID: "tax-event-impact", Description: "tax changes, withholding implications, and deadlines are explicit and grounded"},
		},
		VerificationHints: []ExecutionBlockVerificationHint{
			{Rule: "tax_event_grounding", Description: "tax recommendations must cite tax, payroll, or event evidence"},
		},
		RiskHints: []ExecutionBlockRiskHint{
			{Level: "medium", Rationale: "life events frequently create tax follow-up obligations and deadline-sensitive work"},
		},
	}
}

func portfolioEventBlock(spec taskspec.TaskSpec, dependsOn []ExecutionBlockDependency) ExecutionBlock {
	return ExecutionBlock{
		ID:                ExecutionBlockID("portfolio-event-impact"),
		Kind:              ExecutionBlockKindPortfolioEventImpact,
		AssignedRecipient: BlockRecipientPortfolioAgent,
		Goal:              "评估 life event 对配置漂移、流动性缓冲和再平衡动作的影响。",
		RequiredEvidenceRefs: requiredByType(spec.RequiredEvidence,
			"event_signal",
			"portfolio_allocation_snapshot",
			"transaction_batch",
		),
		RequiredMemoryKinds:  []memory.MemoryKind{memory.MemoryKindSemantic, memory.MemoryKindEpisodic},
		RequiredStateBlocks:  []string{"portfolio_state", "cashflow_state", "risk_state"},
		ExecutionContextView: contextview.ContextViewExecution,
		SuccessCriteria: []ExecutionBlockSuccessCriteria{
			{ID: "portfolio-event-impact", Description: "portfolio drift, liquidity tradeoff, and rebalance implications are explicit and grounded"},
		},
		VerificationHints: []ExecutionBlockVerificationHint{
			{Rule: "portfolio_event_grounding", Description: "portfolio recommendations must cite portfolio and event evidence"},
		},
		RiskHints: []ExecutionBlockRiskHint{
			{Level: "medium", Rationale: "event-driven cashflow changes may alter allocation drift tolerance and liquidity needs"},
		},
		DependsOn: dependsOn,
	}
}

func requiredByType(items []taskspec.RequiredEvidenceRef, allowedTypes ...string) []ExecutionBlockRequirement {
	allowed := make(map[string]struct{}, len(allowedTypes))
	for _, item := range allowedTypes {
		allowed[item] = struct{}{}
	}
	result := make([]ExecutionBlockRequirement, 0, len(items))
	for _, item := range items {
		if _, ok := allowed[item.Type]; !ok {
			continue
		}
		result = append(result, ExecutionBlockRequirement{
			RequirementID: item.Type,
			Type:          item.Type,
			Mandatory:     item.Mandatory,
		})
	}
	return result
}

func hasDebtPriorityMemory(slice contextview.ContextSlice) bool {
	for _, item := range slice.MemoryBlocks {
		summary := strings.ToLower(item.Summary)
		if strings.Contains(summary, "debt pressure") ||
			strings.Contains(summary, "debt-versus-invest") ||
			strings.Contains(summary, "debt burden") {
			return true
		}
	}
	return false
}

func hasTaxPriorityMemory(slice contextview.ContextSlice) bool {
	for _, item := range slice.MemoryBlocks {
		summary := strings.ToLower(item.Summary)
		if strings.Contains(summary, "tax signal") ||
			strings.Contains(summary, "family-related tax") ||
			strings.Contains(summary, "withholding") {
			return true
		}
	}
	return false
}

func taskHasScopeNote(spec taskspec.TaskSpec, note string) bool {
	for _, item := range spec.Scope.Notes {
		if strings.EqualFold(item, note) {
			return true
		}
	}
	return false
}
