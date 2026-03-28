package workflows

import (
	"context"
	"fmt"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/agents"
	"github.com/kobelakers/personal-cfo-os/internal/governance"
	runtimepkg "github.com/kobelakers/personal-cfo-os/internal/runtime"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
	"github.com/kobelakers/personal-cfo-os/internal/verification"
)

type DebtVsInvestWorkflow struct {
	Intake          taskspec.DeterministicIntakeService
	DecisionService DebtVsInvestService
	SystemSteps     agents.SystemStepBus
	Runtime         runtimepkg.WorkflowRuntime
	Now             func() time.Time
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

	steps := w.systemSteps()
	if steps == nil {
		return DebtDecisionRunResult{}, fmt.Errorf("debt-vs-invest workflow requires system step bus")
	}

	meta := systemStepMeta(workflowID, "debt_vs_invest_workflow", spec, observed.UpdatedState, workflowID, workflowID)

	planStep, err := steps.DispatchPlan(ctx, meta, observed.UpdatedState, nil, observed.Evidence)
	if err != nil {
		return DebtDecisionRunResult{}, handleAgentFailure(workflowRuntime, execCtx, runtimepkg.WorkflowStatePlanning, err, "planner agent failed")
	}
	meta = updateCausation(meta, planStep.Metadata.ResponseMetadata, observed.UpdatedState)

	memoryStep, err := steps.DispatchMemorySync(ctx, meta, observed.UpdatedState, observed.Evidence, "")
	if err != nil {
		return DebtDecisionRunResult{}, handleAgentFailure(workflowRuntime, execCtx, runtimepkg.WorkflowStateActing, err, "memory steward failed")
	}
	meta = updateCausation(meta, memoryStep.Metadata.ResponseMetadata, observed.UpdatedState)

	reportDraftStep, err := steps.DispatchReportDraft(ctx, meta, observed.UpdatedState, memoryStep.Result.Retrieved, observed.Evidence, planStep.Plan)
	if err != nil {
		return DebtDecisionRunResult{}, handleAgentFailure(workflowRuntime, execCtx, runtimepkg.WorkflowStateActing, err, "report draft generation failed")
	}
	report, err := debtDecisionReportFromPayload(reportDraftStep.Draft)
	if err != nil {
		return DebtDecisionRunResult{}, err
	}
	meta = updateCausation(meta, reportDraftStep.Metadata.ResponseMetadata, observed.UpdatedState)

	verificationStep, err := steps.DispatchVerification(ctx, meta, observed.UpdatedState, observed.Evidence, reportDraftStep.Draft)
	if err != nil {
		return DebtDecisionRunResult{}, handleAgentFailure(workflowRuntime, execCtx, runtimepkg.WorkflowStateVerifying, err, "verification agent failed")
	}
	meta = updateCausation(meta, verificationStep.Metadata.ResponseMetadata, observed.UpdatedState)

	runtimeState := runtimepkg.WorkflowStateCompleted
	shouldReplan := verification.NeedsReplan(verificationStep.Result.Results)
	if shouldReplan {
		nextState, _, err := workflowRuntime.HandleFailure(execCtx, runtimepkg.WorkflowStateVerifying, runtimepkg.FailureCategoryValidation, "decision verification failed; workflow should replan")
		if err != nil {
			return DebtDecisionRunResult{}, err
		}
		runtimeState = nextState
		return DebtDecisionRunResult{
			WorkflowID:     workflowID,
			Intake:         intake,
			TaskSpec:       spec,
			Plan:           planStep.Plan,
			Evidence:       observed.Evidence,
			UpdatedState:   observed.UpdatedState,
			Report:         report,
			CoverageReport: verificationStep.Result.CoverageReport,
			Verification:   verificationStep.Result.Results,
			Oracle:         verificationStep.Result.OracleVerdict,
			RuntimeState:   runtimeState,
		}, nil
	}

	governanceStep, err := steps.DispatchGovernance(ctx, meta, observed.UpdatedState, reportDraftStep.Draft)
	if err != nil {
		return DebtDecisionRunResult{}, handleAgentFailure(workflowRuntime, execCtx, runtimepkg.WorkflowStateVerifying, err, "governance agent failed")
	}
	meta = updateCausation(meta, governanceStep.Metadata.ResponseMetadata, observed.UpdatedState)

	var artifacts []WorkflowArtifact
	var approvalDecision *governance.PolicyDecision
	var approvalAudit *governance.AuditEvent
	if governanceStep.Approval.Decision != nil {
		approvalDecision = governanceStep.Approval.Decision
	}
	if governanceStep.Approval.Audit != nil {
		approvalAudit = governanceStep.Approval.Audit
	}

	switch {
	case approvalDecision != nil && approvalDecision.Outcome == governance.PolicyDecisionRequireApproval:
		nextState, err := workflowRuntime.PauseForApproval(execCtx, runtimepkg.WorkflowStateVerifying, runtimepkg.HumanApprovalPending{
			ApprovalID:      workflowID + "-approval",
			WorkflowID:      workflowID,
			RequestedAction: "debt_vs_invest_recommendation",
			RequiredRoles:   approvalRoles(governanceStep.Approval),
			RequestedAt:     now,
		})
		if err != nil {
			return DebtDecisionRunResult{}, err
		}
		runtimeState = nextState
		report.ApprovalRequired = true
	case approvalDecision != nil && approvalDecision.Outcome == governance.PolicyDecisionDeny:
		nextState, _, err := workflowRuntime.HandleFailure(execCtx, runtimepkg.WorkflowStateVerifying, runtimepkg.FailureCategoryUnrecoverable, "governance denied debt decision publication")
		if err != nil {
			return DebtDecisionRunResult{}, err
		}
		runtimeState = nextState
	case governanceStep.Disclosure.Decision.Outcome == governance.PolicyDecisionAllow || governanceStep.Disclosure.Decision.Outcome == governance.PolicyDecisionRedact:
		finalizeStep, err := steps.DispatchReportFinalize(ctx, meta, reportDraftStep.Draft, governanceStep.Disclosure.Decision)
		if err != nil {
			return DebtDecisionRunResult{}, handleAgentFailure(workflowRuntime, execCtx, runtimepkg.WorkflowStateVerifying, err, "report finalization failed")
		}
		finalReport, err := debtDecisionReportFromPayload(finalizeStep.Report)
		if err != nil {
			return DebtDecisionRunResult{}, err
		}
		report = finalReport
		artifacts = finalizeStep.Artifacts
	default:
		runtimeState = runtimepkg.WorkflowStateFailed
	}

	return DebtDecisionRunResult{
		WorkflowID:       workflowID,
		Intake:           intake,
		TaskSpec:         spec,
		Plan:             planStep.Plan,
		Evidence:         observed.Evidence,
		UpdatedState:     observed.UpdatedState,
		Report:           report,
		Artifacts:        artifacts,
		CoverageReport:   verificationStep.Result.CoverageReport,
		Verification:     verificationStep.Result.Results,
		Oracle:           verificationStep.Result.OracleVerdict,
		RiskAssessment:   governanceStep.Approval.RiskAssessment,
		ApprovalDecision: approvalDecision,
		ApprovalAudit:    approvalAudit,
		RuntimeState:     runtimeState,
	}, nil
}

func (w DebtVsInvestWorkflow) systemSteps() agents.SystemStepBus {
	return w.SystemSteps
}

func (w DebtVsInvestWorkflow) now() time.Time {
	if w.Now != nil {
		return w.Now().UTC()
	}
	return time.Now().UTC()
}
