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
	WorkflowID string     `json:"workflow_id"`
	TaskID     string     `json:"task_id"`
	CreatedAt  time.Time  `json:"created_at"`
	Steps      []PlanStep `json:"steps"`
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

	switch spec.UserIntentType {
	case taskspec.UserIntentMonthlyReview:
		return ExecutionPlan{
			WorkflowID: workflowID,
			TaskID:     spec.ID,
			CreatedAt:  now,
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
		return ExecutionPlan{
			WorkflowID: workflowID,
			TaskID:     spec.ID,
			CreatedAt:  now,
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
	default:
		return ExecutionPlan{
			WorkflowID: workflowID,
			TaskID:     spec.ID,
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
