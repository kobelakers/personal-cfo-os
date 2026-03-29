package planning

import (
	"fmt"
)

func (p ExecutionPlan) Validate() error {
	if p.WorkflowID == "" {
		return fmt.Errorf("execution plan workflow_id is required")
	}
	if p.TaskID == "" {
		return fmt.Errorf("execution plan task_id is required")
	}
	if p.CreatedAt.IsZero() {
		return fmt.Errorf("execution plan created_at is required")
	}
	if len(p.Blocks) == 0 {
		return fmt.Errorf("execution plan requires at least one execution block")
	}

	seen := make(map[ExecutionBlockID]ExecutionBlock, len(p.Blocks))
	for _, block := range p.Blocks {
		if err := block.Validate(); err != nil {
			return fmt.Errorf("execution block %q invalid: %w", block.ID, err)
		}
		if _, ok := seen[block.ID]; ok {
			return fmt.Errorf("duplicate execution block id %q", block.ID)
		}
		seen[block.ID] = block
	}
	for _, block := range p.Blocks {
		for _, dep := range block.DependsOn {
			if _, ok := seen[dep.BlockID]; !ok {
				return fmt.Errorf("execution block %q depends on unknown block %q", block.ID, dep.BlockID)
			}
			if dep.BlockID == block.ID {
				return fmt.Errorf("execution block %q cannot depend on itself", block.ID)
			}
		}
	}
	visiting := make(map[ExecutionBlockID]bool, len(p.Blocks))
	visited := make(map[ExecutionBlockID]bool, len(p.Blocks))
	var visit func(ExecutionBlockID) error
	visit = func(id ExecutionBlockID) error {
		if visited[id] {
			return nil
		}
		if visiting[id] {
			return fmt.Errorf("cyclic dependency detected at %q", id)
		}
		visiting[id] = true
		for _, dep := range seen[id].DependsOn {
			if err := visit(dep.BlockID); err != nil {
				return err
			}
		}
		visiting[id] = false
		visited[id] = true
		return nil
	}
	for _, block := range p.Blocks {
		if err := visit(block.ID); err != nil {
			return err
		}
	}
	return nil
}

func (b ExecutionBlock) Validate() error {
	if b.ID == "" {
		return fmt.Errorf("block id is required")
	}
	if b.Kind == "" {
		return fmt.Errorf("block kind is required")
	}
	if b.AssignedRecipient == "" {
		return fmt.Errorf("assigned recipient is required")
	}
	if err := validateBlockRecipient(b.Kind, b.AssignedRecipient); err != nil {
		return err
	}
	if b.Goal == "" {
		return fmt.Errorf("block goal is required")
	}
	if len(b.RequiredEvidenceRefs) == 0 {
		return fmt.Errorf("block requires at least one evidence requirement")
	}
	mandatoryCount := 0
	for _, req := range b.RequiredEvidenceRefs {
		if req.RequirementID == "" || req.Type == "" {
			return fmt.Errorf("block evidence requirements must include requirement id and type")
		}
		if req.Mandatory {
			mandatoryCount++
		}
	}
	if mandatoryCount == 0 {
		return fmt.Errorf("block must include at least one mandatory evidence requirement")
	}
	if b.ExecutionContextView == "" {
		return fmt.Errorf("execution context view is required")
	}
	if len(b.SuccessCriteria) == 0 {
		return fmt.Errorf("block must include success criteria")
	}
	for _, item := range b.SuccessCriteria {
		if item.ID == "" || item.Description == "" {
			return fmt.Errorf("success criteria must include id and description")
		}
	}
	if len(b.VerificationHints) == 0 {
		return fmt.Errorf("block must include verification hints")
	}
	for _, item := range b.VerificationHints {
		if item.Rule == "" || item.Description == "" {
			return fmt.Errorf("verification hints must include rule and description")
		}
	}
	return nil
}

func validateBlockRecipient(kind ExecutionBlockKind, recipient string) error {
	switch kind {
	case ExecutionBlockKindCashflowReview, ExecutionBlockKindCashflowLiquidity, ExecutionBlockKindCashflowEventImpact:
		if recipient != BlockRecipientCashflowAgent {
			return fmt.Errorf("cashflow block %q must be assigned to %q", kind, BlockRecipientCashflowAgent)
		}
	case ExecutionBlockKindDebtReview, ExecutionBlockKindDebtTradeoff, ExecutionBlockKindDebtHousingImpact:
		if recipient != BlockRecipientDebtAgent {
			return fmt.Errorf("debt block %q must be assigned to %q", kind, BlockRecipientDebtAgent)
		}
	case ExecutionBlockKindTaxEventImpact, ExecutionBlockKindTaxOptimization:
		if recipient != BlockRecipientTaxAgent {
			return fmt.Errorf("tax block %q must be assigned to %q", kind, BlockRecipientTaxAgent)
		}
	case ExecutionBlockKindPortfolioEventImpact, ExecutionBlockKindPortfolioRebalance:
		if recipient != BlockRecipientPortfolioAgent {
			return fmt.Errorf("portfolio block %q must be assigned to %q", kind, BlockRecipientPortfolioAgent)
		}
	default:
		return fmt.Errorf("unsupported execution block kind %q", kind)
	}
	return nil
}
