package planning

import (
	"time"

	contextview "github.com/kobelakers/personal-cfo-os/internal/context"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

type PlanStep struct {
	ID                    string   `json:"id"`
	Name                  string   `json:"name"`
	Description           string   `json:"description"`
	RequiredEvidenceTypes []string `json:"required_evidence_types,omitempty"`
	RequiredTools         []string `json:"required_tools,omitempty"`
	RequiredSkills        []string `json:"required_skills,omitempty"`
}

type ExecutionPlan struct {
	WorkflowID string           `json:"workflow_id"`
	TaskID     string           `json:"task_id"`
	PlanID     string           `json:"plan_id"`
	CreatedAt  time.Time        `json:"created_at"`
	Blocks     []ExecutionBlock `json:"blocks"`
	Steps      []PlanStep       `json:"steps"`
}

type DeterministicPlanner struct {
	PlanStateMachine
	Task *taskspec.TaskSpec
	Now  func() time.Time
}

func (p *DeterministicPlanner) BindTask(spec taskspec.TaskSpec) error {
	p.Task = &spec
	return p.PlanStateMachine.BindTask(spec)
}

func (p *DeterministicPlanner) CreatePlan(spec taskspec.TaskSpec, slice contextview.ContextSlice, workflowID string) ExecutionPlan {
	_ = p.BindTask(spec)
	now := time.Now().UTC()
	if p.Now != nil {
		now = p.Now().UTC()
	}

	planID := workflowID + "-plan-" + now.Format("20060102150405")

	switch spec.UserIntentType {
	case taskspec.UserIntentMonthlyReview:
		blocks := monthlyReviewBlocks(spec, slice)
		return ExecutionPlan{
			WorkflowID: workflowID,
			TaskID:     spec.ID,
			PlanID:     planID,
			CreatedAt:  now,
			Blocks:     blocks,
			Steps: []PlanStep{
				{
					ID:                    "collect-and-confirm",
					Name:                  "确认证据覆盖",
					Description:           "检查交易、债务、持仓、税务相关证据是否完整，并将缺口标记给后续验证阶段。",
					RequiredEvidenceTypes: requiredEvidenceTypes(spec.RequiredEvidence),
				},
				{
					ID:             "compute-metrics",
					Name:           "计算月度指标",
					Description:    "基于状态块与证据摘要计算现金流、债务、税务和行为信号。",
					RequiredTools:  []string{"compute_cashflow_metrics_tool", "compute_tax_signal_tool"},
					RequiredSkills: []string{"monthly_review"},
				},
				{
					ID:             "assemble-report",
					Name:           "生成结构化复盘报告",
					Description:    "将风险点、优化建议和待办项组装为结构化报告，并产出 artifacts。",
					RequiredTools:  []string{"generate_task_artifact_tool"},
					RequiredSkills: []string{"monthly_review"},
				},
			},
		}
	case taskspec.UserIntentDebtVsInvest:
		blocks := debtDecisionBlocks(spec, slice)
		return ExecutionPlan{
			WorkflowID: workflowID,
			TaskID:     spec.ID,
			PlanID:     planID,
			CreatedAt:  now,
			Blocks:     blocks,
			Steps: []PlanStep{
				{
					ID:                    "gather-decision-evidence",
					Name:                  "收集债务与投资证据",
					Description:           "拉取债务、现金流和持仓证据，形成决策基线。",
					RequiredEvidenceTypes: requiredEvidenceTypes(spec.RequiredEvidence),
				},
				{
					ID:             "compute-decision-metrics",
					Name:           "计算对比指标",
					Description:    "使用确定性指标比较提前还贷与继续投资的风险和现金流影响。",
					RequiredTools:  []string{"compute_debt_decision_metrics_tool"},
					RequiredSkills: []string{"debt_optimization"},
				},
			},
		}
	case taskspec.UserIntentLifeEventTrigger:
		blocks := lifeEventBlocks(spec, slice)
		return ExecutionPlan{
			WorkflowID: workflowID,
			TaskID:     spec.ID,
			PlanID:     planID,
			CreatedAt:  now,
			Blocks:     blocks,
			Steps: []PlanStep{
				{
					ID:                    "observe-life-event",
					Name:                  "归一化 life event 证据",
					Description:           "事件与 deadline 先归一化为 typed evidence，再驱动状态更新和 block plan。",
					RequiredEvidenceTypes: requiredEvidenceTypes(spec.RequiredEvidence),
				},
				{
					ID:             "analyze-event-impact",
					Name:           "执行事件影响分析块",
					Description:    "按照 plan.Blocks 顺序执行现金流、债务、税务和配置影响分析。",
					RequiredTools:  []string{"query_event_tool", "query_calendar_deadline_tool", "compute_tax_signal_tool"},
					RequiredSkills: []string{"life_event_trigger"},
				},
				{
					ID:             "spawn-follow-up",
					Name:           "生成后续财务任务图",
					Description:    "基于已验证的事件分析结果生成 typed follow-up tasks 并注册到 runtime。",
					RequiredTools:  []string{"generate_task_artifact_tool"},
					RequiredSkills: []string{"life_event_trigger"},
				},
			},
		}
	case taskspec.UserIntentTaxOptimization:
		blocks := taxOptimizationBlocks(spec)
		return ExecutionPlan{
			WorkflowID: workflowID,
			TaskID:     spec.ID,
			PlanID:     planID,
			CreatedAt:  now,
			Blocks:     blocks,
			Steps: []PlanStep{
				{
					ID:                    "collect-tax-follow-up-evidence",
					Name:                  "收集 tax follow-up 证据",
					Description:           "拉取与本次 life event 相关的 event、deadline、tax document 和 payroll evidence。",
					RequiredEvidenceTypes: requiredEvidenceTypes(spec.RequiredEvidence),
				},
				{
					ID:             "analyze-tax-optimization",
					Name:           "执行 tax optimization block",
					Description:    "使用单 block tax analysis 生成 grounded follow-up optimization output。",
					RequiredTools:  []string{"query_event_tool", "query_calendar_deadline_tool", "compute_tax_signal_tool"},
					RequiredSkills: []string{"tax_optimization"},
				},
			},
		}
	case taskspec.UserIntentPortfolioRebalance:
		blocks := portfolioRebalanceBlocks(spec)
		return ExecutionPlan{
			WorkflowID: workflowID,
			TaskID:     spec.ID,
			PlanID:     planID,
			CreatedAt:  now,
			Blocks:     blocks,
			Steps: []PlanStep{
				{
					ID:                    "collect-portfolio-follow-up-evidence",
					Name:                  "收集 portfolio follow-up 证据",
					Description:           "拉取与本次 life event 相关的 event、holdings 和 liquidity baseline evidence。",
					RequiredEvidenceTypes: requiredEvidenceTypes(spec.RequiredEvidence),
				},
				{
					ID:             "analyze-portfolio-rebalance",
					Name:           "执行 portfolio rebalance block",
					Description:    "使用单 block portfolio analysis 生成 grounded rebalance output。",
					RequiredTools:  []string{"query_event_tool", "query_portfolio_tool", "compute_portfolio_impact_metrics_tool"},
					RequiredSkills: []string{"portfolio_rebalance"},
				},
			},
		}
	default:
		return ExecutionPlan{
			WorkflowID: workflowID,
			TaskID:     spec.ID,
			PlanID:     planID,
			CreatedAt:  now,
		}
	}
}

func requiredEvidenceTypes(items []taskspec.RequiredEvidenceRef) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, item.Type)
	}
	return result
}
