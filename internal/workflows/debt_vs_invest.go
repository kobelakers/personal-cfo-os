package workflows

import (
	"context"
	"fmt"
	"time"

	contextview "github.com/kobelakers/personal-cfo-os/internal/context"
	"github.com/kobelakers/personal-cfo-os/internal/governance"
	"github.com/kobelakers/personal-cfo-os/internal/memory"
	"github.com/kobelakers/personal-cfo-os/internal/planning"
	runtimepkg "github.com/kobelakers/personal-cfo-os/internal/runtime"
	"github.com/kobelakers/personal-cfo-os/internal/skills"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
	"github.com/kobelakers/personal-cfo-os/internal/tools"
	"github.com/kobelakers/personal-cfo-os/internal/verification"
)

type DebtVsInvestWorkflow struct {
	Intake               taskspec.DeterministicIntakeService
	DecisionService      DebtVsInvestService
	MemoryService        memory.WorkflowMemoryService
	MemoryWritePolicy    governance.MemoryWritePolicy
	ContextAssembler     contextview.ContextAssembler
	Planner              *planning.DeterministicPlanner
	Skill                skills.DebtOptimizationSkill
	ComputeMetrics       tools.ComputeDebtDecisionMetricsTool
	ArtifactService      ArtifactService
	VerificationPipeline verification.Pipeline
	ApprovalService      governance.ApprovalService
	Runtime              runtimepkg.WorkflowRuntime
	Now                  func() time.Time
}

func (w DebtVsInvestWorkflow) Run(
	ctx context.Context,
	userID string,
	rawInput string,
	current state.FinancialWorldState,
) (DebtDecisionRunResult, error) {
	intake := w.Intake.Parse(rawInput)
	if !intake.Accepted || intake.TaskSpec == nil {
		return DebtDecisionRunResult{Intake: intake}, fmt.Errorf("task intake rejected: %s", intake.FailureReason)
	}
	spec := *intake.TaskSpec
	now := w.now()
	if current.UserID == "" {
		current.UserID = userID
	}
	workflowID := "workflow-debt-vs-invest-" + now.Format("20060102150405")
	execCtx := runtimepkg.ExecutionContext{
		WorkflowID:    workflowID,
		TaskID:        spec.ID,
		CorrelationID: workflowID,
		Attempt:       1,
	}
	workflowRuntime := runtimepkg.ResolveWorkflowRuntime(w.Runtime, workflowID, w.Now)

	observed, err := w.DecisionService.ObserveAndReduce(ctx, spec, userID, workflowID, current)
	if err != nil {
		return DebtDecisionRunResult{}, err
	}
	if _, _, err = workflowRuntime.Checkpoint(execCtx, runtimepkg.WorkflowStatePlanning, runtimepkg.WorkflowStatePlanning, current.Version.Sequence, "decision evidence collected"); err != nil {
		return DebtDecisionRunResult{}, err
	}
	if _, _, err = workflowRuntime.Checkpoint(execCtx, runtimepkg.WorkflowStateActing, runtimepkg.WorkflowStateActing, observed.Diff.ToVersion, "decision state updated"); err != nil {
		return DebtDecisionRunResult{}, err
	}

	assembler := w.contextAssembler()
	planningContext, err := assembler.Assemble(spec, observed.UpdatedState, nil, observed.Evidence, contextview.ContextViewPlanning)
	if err != nil {
		return DebtDecisionRunResult{}, err
	}
	planner := w.planner()
	plan := planner.CreatePlan(spec, planningContext, workflowID)
	if _, err = assembler.Assemble(spec, observed.UpdatedState, nil, observed.Evidence, contextview.ContextViewExecution); err != nil {
		return DebtDecisionRunResult{}, err
	}

	skillOutput := w.Skill.Analyze(observed.UpdatedState)
	report := DebtDecisionReport{
		TaskID:           spec.ID,
		WorkflowID:       workflowID,
		Conclusion:       skillOutput.Conclusion,
		Reasons:          skillOutput.Reasons,
		Actions:          skillOutput.Actions,
		Metrics:          w.ComputeMetrics.Compute(observed.UpdatedState),
		EvidenceIDs:      collectEvidenceIDs(observed.Evidence),
		ApprovalRequired: observed.UpdatedState.RiskState.OverallRisk == "high",
		Confidence:       skillOutput.Confidence,
		GeneratedAt:      now,
	}

	memoryResult, err := w.memoryService(workflowID).SyncDebtDecision(ctx, spec, workflowID, observed.UpdatedState, observed.Evidence, report.Conclusion)
	if err != nil {
		return DebtDecisionRunResult{}, err
	}
	if _, err = assembler.Assemble(spec, observed.UpdatedState, memoryResult.Retrieved, observed.Evidence, contextview.ContextViewVerification); err != nil {
		return DebtDecisionRunResult{}, err
	}

	reportArtifact, err := w.artifacts().Produce(workflowID, spec.ID, ArtifactKindDebtDecisionReport, report, report.Conclusion, w.Skill.Name())
	if err != nil {
		return DebtDecisionRunResult{}, err
	}
	verificationResult, err := w.verificationPipeline().VerifyDebtDecision(ctx, spec, observed.UpdatedState, observed.Evidence, report)
	if err != nil {
		return DebtDecisionRunResult{}, err
	}

	approvalEvaluation, err := w.approvals().EvaluateAction(observed.UpdatedState, workflowID, "debt_vs_invest_recommendation", report.TaskID, "governance_agent", []string{"analyst"}, report.ApprovalRequired)
	if err != nil {
		return DebtDecisionRunResult{}, err
	}
	if approvalEvaluation.Decision != nil && approvalEvaluation.Decision.Outcome == governance.PolicyDecisionRequireApproval {
		report.ApprovalRequired = true
	}
	reportEvaluation, err := w.approvals().EvaluateReport(workflowID, "report_agent", "user", false)
	if err != nil {
		return DebtDecisionRunResult{}, err
	}
	if reportEvaluation.Decision.Outcome == governance.PolicyDecisionRedact {
		report.Conclusion = "[REDACTED] " + report.Conclusion
	}

	runtimeState := runtimepkg.WorkflowStateCompleted
	if verification.NeedsReplan(verificationResult.Results) {
		nextState, _, err := workflowRuntime.HandleFailure(execCtx, runtimepkg.WorkflowStateVerifying, runtimepkg.FailureCategoryValidation, "decision verification failed; workflow should replan")
		if err != nil {
			return DebtDecisionRunResult{}, err
		}
		runtimeState = nextState
	}
	if approvalEvaluation.Decision != nil && approvalEvaluation.Decision.Outcome == governance.PolicyDecisionRequireApproval {
		nextState, err := workflowRuntime.PauseForApproval(execCtx, runtimepkg.WorkflowStateVerifying, runtimepkg.HumanApprovalPending{
			ApprovalID:      workflowID + "-approval",
			WorkflowID:      workflowID,
			RequestedAction: "debt_vs_invest_recommendation",
			RequiredRoles:   w.approvals().ApprovalPolicy.RequiredRoles,
			RequestedAt:     now,
		})
		if err != nil {
			return DebtDecisionRunResult{}, err
		}
		runtimeState = nextState
	}

	return DebtDecisionRunResult{
		WorkflowID:       workflowID,
		Intake:           intake,
		TaskSpec:         spec,
		Plan:             plan,
		Evidence:         observed.Evidence,
		UpdatedState:     observed.UpdatedState,
		Report:           report,
		Artifacts:        []WorkflowArtifact{reportArtifact},
		CoverageReport:   verificationResult.CoverageReport,
		Verification:     verificationResult.Results,
		Oracle:           verificationResult.OracleVerdict,
		RiskAssessment:   approvalEvaluation.RiskAssessment,
		ApprovalDecision: approvalEvaluation.Decision,
		ApprovalAudit:    approvalEvaluation.Audit,
		RuntimeState:     runtimeState,
	}, nil
}

func (w DebtVsInvestWorkflow) contextAssembler() contextview.ContextAssembler {
	if w.ContextAssembler != nil {
		return w.ContextAssembler
	}
	return contextview.DefaultContextAssembler{}
}

func (w DebtVsInvestWorkflow) planner() *planning.DeterministicPlanner {
	if w.Planner != nil {
		return w.Planner
	}
	return &planning.DeterministicPlanner{}
}

func (w DebtVsInvestWorkflow) artifacts() ArtifactService {
	service := w.ArtifactService
	if service.Now == nil {
		service.Now = w.Now
	}
	return service
}

func (w DebtVsInvestWorkflow) approvals() governance.ApprovalService {
	return w.ApprovalService
}

func (w DebtVsInvestWorkflow) verificationPipeline() verification.Pipeline {
	return w.VerificationPipeline
}

func (w DebtVsInvestWorkflow) memoryService(workflowID string) memory.WorkflowMemoryService {
	service := w.MemoryService
	if service.Now == nil {
		service.Now = w.Now
	}
	if service.Gate == nil {
		service.Gate = governance.MemoryWriteGateService{
			PolicyEngine:  w.ApprovalService.PolicyEngine,
			Policy:        w.memoryWritePolicy(),
			CorrelationID: workflowID,
		}
	}
	return service
}

func (w DebtVsInvestWorkflow) memoryWritePolicy() governance.MemoryWritePolicy {
	if w.MemoryWritePolicy.MinConfidence != 0 || w.MemoryWritePolicy.RequireEvidence || len(w.MemoryWritePolicy.AllowKinds) > 0 {
		return w.MemoryWritePolicy
	}
	return governance.MemoryWritePolicy{
		MinConfidence:   0.7,
		RequireEvidence: false,
		AllowKinds: []memory.MemoryKind{
			memory.MemoryKindEpisodic,
			memory.MemoryKindSemantic,
			memory.MemoryKindProcedural,
		},
	}
}

func (w DebtVsInvestWorkflow) now() time.Time {
	if w.Now != nil {
		return w.Now().UTC()
	}
	return time.Now().UTC()
}
