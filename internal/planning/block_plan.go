package planning

import (
	"strings"
	"time"

	contextview "github.com/kobelakers/personal-cfo-os/internal/context"
	"github.com/kobelakers/personal-cfo-os/internal/memory"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

const (
	BlockRecipientCashflowAgent = "cashflow_agent"
	BlockRecipientDebtAgent     = "debt_agent"
)

type ExecutionBlockID string
type ExecutionBlockKind string

const (
	ExecutionBlockKindCashflowReview    ExecutionBlockKind = "cashflow_review_block"
	ExecutionBlockKindDebtReview        ExecutionBlockKind = "debt_review_block"
	ExecutionBlockKindCashflowLiquidity ExecutionBlockKind = "cashflow_liquidity_block"
	ExecutionBlockKindDebtTradeoff      ExecutionBlockKind = "debt_tradeoff_block"
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
