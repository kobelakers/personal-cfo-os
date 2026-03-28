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

type MonthlyReviewWorkflow struct {
	Intake        taskspec.DeterministicIntakeService
	ReviewService MonthlyReviewService
	SystemSteps   agents.SystemStepBus
	Runtime       runtimepkg.WorkflowRuntime
	Now           func() time.Time
}

func (w MonthlyReviewWorkflow) Run(
	ctx context.Context,
	userID string,
	rawInput string,
	current state.FinancialWorldState,
) (MonthlyReviewRunResult, error) {
	intake := w.Intake.Parse(rawInput)
	if !intake.Accepted || intake.TaskSpec == nil {
		return MonthlyReviewRunResult{Intake: intake}, fmt.Errorf("task intake rejected: %s", intake.FailureReason)
	}
	spec := *intake.TaskSpec
	now := w.now()
	if current.UserID == "" {
		current.UserID = userID
	}

	workflowID := "workflow-monthly-review-" + now.Format("20060102150405")
	execCtx := runtimepkg.ExecutionContext{
		WorkflowID:    workflowID,
		TaskID:        spec.ID,
		CorrelationID: workflowID,
		Attempt:       1,
	}
	workflowRuntime := runtimepkg.ResolveWorkflowRuntime(w.Runtime, workflowID, w.Now)

	observed, err := w.ReviewService.ObserveAndReduce(ctx, spec, userID, workflowID, current)
	if err != nil {
		return MonthlyReviewRunResult{}, err
	}
	if _, _, err = workflowRuntime.Checkpoint(execCtx, runtimepkg.WorkflowStatePlanning, runtimepkg.WorkflowStatePlanning, current.Version.Sequence, "observation completed"); err != nil {
		return MonthlyReviewRunResult{}, err
	}
	if _, _, err = workflowRuntime.Checkpoint(execCtx, runtimepkg.WorkflowStateActing, runtimepkg.WorkflowStateActing, observed.Diff.ToVersion, "state updated from evidence patch"); err != nil {
		return MonthlyReviewRunResult{}, err
	}

	steps := w.systemSteps()
	if steps == nil {
		return MonthlyReviewRunResult{}, fmt.Errorf("monthly review workflow requires system step bus")
	}

	meta := systemStepMeta(workflowID, "monthly_review_workflow", spec, observed.UpdatedState, workflowID, workflowID)

	planStep, err := steps.DispatchPlan(ctx, meta, observed.UpdatedState, nil, observed.Evidence)
	if err != nil {
		return MonthlyReviewRunResult{}, handleAgentFailure(workflowRuntime, execCtx, runtimepkg.WorkflowStatePlanning, err, "planner agent failed")
	}
	meta = updateCausation(meta, planStep.Metadata.ResponseMetadata, observed.UpdatedState)

	memoryStep, err := steps.DispatchMemorySync(ctx, meta, observed.UpdatedState, observed.Evidence, "")
	if err != nil {
		return MonthlyReviewRunResult{}, handleAgentFailure(workflowRuntime, execCtx, runtimepkg.WorkflowStateActing, err, "memory steward failed")
	}
	meta = updateCausation(meta, memoryStep.Metadata.ResponseMetadata, observed.UpdatedState)

	reportDraftStep, err := steps.DispatchReportDraft(ctx, meta, observed.UpdatedState, memoryStep.Result.Retrieved, observed.Evidence, planStep.Plan)
	if err != nil {
		return MonthlyReviewRunResult{}, handleAgentFailure(workflowRuntime, execCtx, runtimepkg.WorkflowStateActing, err, "report agent draft failed")
	}
	report, err := monthlyReviewReportFromPayload(reportDraftStep.Draft)
	if err != nil {
		return MonthlyReviewRunResult{}, err
	}
	meta = updateCausation(meta, reportDraftStep.Metadata.ResponseMetadata, observed.UpdatedState)
	if _, _, err = workflowRuntime.Checkpoint(execCtx, runtimepkg.WorkflowStateActing, runtimepkg.WorkflowStateVerifying, observed.UpdatedState.Version.Sequence, "system agent draft completed"); err != nil {
		return MonthlyReviewRunResult{}, err
	}

	verificationStep, err := steps.DispatchVerification(ctx, meta, observed.UpdatedState, observed.Evidence, reportDraftStep.Draft)
	if err != nil {
		return MonthlyReviewRunResult{}, handleAgentFailure(workflowRuntime, execCtx, runtimepkg.WorkflowStateVerifying, err, "verification agent failed")
	}
	meta = updateCausation(meta, verificationStep.Metadata.ResponseMetadata, observed.UpdatedState)

	runtimeState := runtimepkg.WorkflowStateCompleted
	shouldReplan := verification.NeedsReplan(verificationStep.Result.Results)
	if shouldReplan {
		nextState, _, err := workflowRuntime.HandleFailure(execCtx, runtimepkg.WorkflowStateVerifying, runtimepkg.FailureCategoryValidation, "verification failed; workflow should replan")
		if err != nil {
			return MonthlyReviewRunResult{}, err
		}
		runtimeState = nextState
		return MonthlyReviewRunResult{
			WorkflowID:        workflowID,
			Intake:            intake,
			TaskSpec:          spec,
			Plan:              planStep.Plan,
			Evidence:          observed.Evidence,
			UpdatedState:      observed.UpdatedState,
			Report:            report,
			Artifacts:         nil,
			GeneratedMemories: memoryStep.Result.GeneratedIDs,
			CoverageReport:    verificationStep.Result.CoverageReport,
			Verification:      verificationStep.Result.Results,
			Oracle:            verificationStep.Result.OracleVerdict,
			RuntimeState:      runtimeState,
		}, nil
	}

	governanceStep, err := steps.DispatchGovernance(ctx, meta, observed.UpdatedState, reportDraftStep.Draft)
	if err != nil {
		return MonthlyReviewRunResult{}, handleAgentFailure(workflowRuntime, execCtx, runtimepkg.WorkflowStateVerifying, err, "governance agent failed")
	}
	meta = updateCausation(meta, governanceStep.Metadata.ResponseMetadata, observed.UpdatedState)

	var approvalDecision *governance.PolicyDecision
	var approvalAudit *governance.AuditEvent
	if governanceStep.Approval.Decision != nil {
		approvalDecision = governanceStep.Approval.Decision
	}
	if governanceStep.Approval.Audit != nil {
		approvalAudit = governanceStep.Approval.Audit
	}

	var artifacts []WorkflowArtifact
	switch {
	case approvalDecision != nil && approvalDecision.Outcome == governance.PolicyDecisionRequireApproval:
		nextState, err := workflowRuntime.PauseForApproval(execCtx, runtimepkg.WorkflowStateVerifying, runtimepkg.HumanApprovalPending{
			ApprovalID:      workflowID + "-approval",
			WorkflowID:      workflowID,
			RequestedAction: "monthly_review_report",
			RequiredRoles:   approvalRoles(governanceStep.Approval),
			RequestedAt:     now,
		})
		if err != nil {
			return MonthlyReviewRunResult{}, err
		}
		runtimeState = nextState
		report.ApprovalRequired = true
	case approvalDecision != nil && approvalDecision.Outcome == governance.PolicyDecisionDeny:
		nextState, _, err := workflowRuntime.HandleFailure(execCtx, runtimepkg.WorkflowStateVerifying, runtimepkg.FailureCategoryUnrecoverable, "governance denied report publication")
		if err != nil {
			return MonthlyReviewRunResult{}, err
		}
		runtimeState = nextState
	case governanceStep.Disclosure.Decision.Outcome == governance.PolicyDecisionAllow || governanceStep.Disclosure.Decision.Outcome == governance.PolicyDecisionRedact:
		finalizeStep, err := steps.DispatchReportFinalize(ctx, meta, reportDraftStep.Draft, governanceStep.Disclosure.Decision)
		if err != nil {
			return MonthlyReviewRunResult{}, handleAgentFailure(workflowRuntime, execCtx, runtimepkg.WorkflowStateVerifying, err, "report finalization failed")
		}
		finalReport, err := monthlyReviewReportFromPayload(finalizeStep.Report)
		if err != nil {
			return MonthlyReviewRunResult{}, err
		}
		report = finalReport
		artifacts = finalizeStep.Artifacts
	default:
		runtimeState = runtimepkg.WorkflowStateFailed
	}

	return MonthlyReviewRunResult{
		WorkflowID:        workflowID,
		Intake:            intake,
		TaskSpec:          spec,
		Plan:              planStep.Plan,
		Evidence:          observed.Evidence,
		UpdatedState:      observed.UpdatedState,
		Report:            report,
		Artifacts:         artifacts,
		GeneratedMemories: memoryStep.Result.GeneratedIDs,
		CoverageReport:    verificationStep.Result.CoverageReport,
		Verification:      verificationStep.Result.Results,
		Oracle:            verificationStep.Result.OracleVerdict,
		RiskAssessment:    governanceStep.Approval.RiskAssessment,
		ApprovalDecision:  approvalDecision,
		ApprovalAudit:     approvalAudit,
		RuntimeState:      runtimeState,
	}, nil
}

func (w MonthlyReviewWorkflow) systemSteps() agents.SystemStepBus {
	return w.SystemSteps
}

func (w MonthlyReviewWorkflow) now() time.Time {
	if w.Now != nil {
		return w.Now().UTC()
	}
	return time.Now().UTC()
}

func approvalRoles(evaluation governance.ApprovalEvaluation) []string {
	if evaluation.Decision == nil {
		return nil
	}
	return []string{"operator"}
}
