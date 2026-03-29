package workflows

import (
	"context"
	"fmt"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/agents"
	contextview "github.com/kobelakers/personal-cfo-os/internal/context"
	"github.com/kobelakers/personal-cfo-os/internal/governance"
	"github.com/kobelakers/personal-cfo-os/internal/observability"
	"github.com/kobelakers/personal-cfo-os/internal/reporting"
	runtimepkg "github.com/kobelakers/personal-cfo-os/internal/runtime"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
	"github.com/kobelakers/personal-cfo-os/internal/verification"
)

type PortfolioRebalanceWorkflow struct {
	Service     FollowUpWorkflowService
	SystemSteps agents.SystemStepBus
	Runtime     runtimepkg.WorkflowRuntime
	EventLog    *observability.EventLog
	Now         func() time.Time
}

func (w PortfolioRebalanceWorkflow) RunTask(
	ctx context.Context,
	spec taskspec.TaskSpec,
	activation runtimepkg.FollowUpActivationContext,
	current state.FinancialWorldState,
) (PortfolioRebalanceRunResult, error) {
	if spec.UserIntentType != taskspec.UserIntentPortfolioRebalance {
		return PortfolioRebalanceRunResult{}, fmt.Errorf("portfolio rebalance workflow requires portfolio_rebalance task spec")
	}
	now := w.now()
	workflowID := "workflow-portfolio-rebalance-" + now.Format("20060102150405")
	execCtx := runtimepkg.ExecutionContext{
		WorkflowID:    workflowID,
		TaskID:        spec.ID,
		CorrelationID: activation.RootCorrelationID,
		Attempt:       1,
	}
	workflowRuntime := runtimepkg.ResolveWorkflowRuntime(w.Runtime, workflowID, w.Now)
	observed, err := w.Service.ObserveAndReduce(ctx, spec, activation, workflowID, current)
	if err != nil {
		return PortfolioRebalanceRunResult{}, err
	}
	if _, _, err = workflowRuntime.Checkpoint(execCtx, runtimepkg.WorkflowStatePlanning, runtimepkg.WorkflowStatePlanning, current.Version.Sequence, "portfolio rebalance evidence observed"); err != nil {
		return PortfolioRebalanceRunResult{}, err
	}
	if _, _, err = workflowRuntime.Checkpoint(execCtx, runtimepkg.WorkflowStateActing, runtimepkg.WorkflowStateActing, observed.Diff.ToVersion, "portfolio rebalance state updated from evidence patch"); err != nil {
		return PortfolioRebalanceRunResult{}, err
	}

	steps := w.systemSteps()
	if steps == nil {
		return PortfolioRebalanceRunResult{}, fmt.Errorf("portfolio rebalance workflow requires system step bus")
	}
	meta := systemStepMeta(workflowID, "portfolio_rebalance_workflow", spec, observed.UpdatedState, activation.RootCorrelationID, activation.TriggeredByTaskID)

	memoryStep, err := steps.DispatchMemorySync(ctx, meta, observed.UpdatedState, observed.Evidence, "")
	if err != nil {
		return PortfolioRebalanceRunResult{}, handleAgentFailure(workflowRuntime, execCtx, runtimepkg.WorkflowStateActing, err, "portfolio rebalance memory steward failed")
	}
	meta = updateCausation(meta, memoryStep.Metadata.ResponseMetadata, observed.UpdatedState)

	planStep, err := steps.DispatchPlan(ctx, meta, observed.UpdatedState, memoryStep.Result.Retrieved, observed.Evidence)
	if err != nil {
		return PortfolioRebalanceRunResult{}, handleAgentFailure(workflowRuntime, execCtx, runtimepkg.WorkflowStatePlanning, err, "portfolio rebalance planner failed")
	}
	meta = updateCausation(meta, planStep.Metadata.ResponseMetadata, observed.UpdatedState)

	executionAssembler := contextview.ExecutionContextAssembler{}
	blockSteps := make([]agents.AnalysisBlockStepResult, 0, len(planStep.Plan.Blocks))
	for _, block := range planStep.Plan.Blocks {
		executionContext, err := executionAssembler.Assemble(blockContextSpec(planStep.Plan, block), observed.UpdatedState, memoryStep.Result.Retrieved, observed.Evidence)
		if err != nil {
			return PortfolioRebalanceRunResult{}, err
		}
		blockStep, err := steps.DispatchAnalysisBlock(ctx, meta, block, observed.UpdatedState, memoryStep.Result.Retrieved, observed.Evidence, executionContext)
		if err != nil {
			return PortfolioRebalanceRunResult{}, handleAgentFailure(workflowRuntime, execCtx, runtimepkg.WorkflowStateActing, err, "portfolio rebalance domain block failed")
		}
		blockSteps = append(blockSteps, blockStep)
		meta = updateCausation(meta, blockStep.Metadata.ResponseMetadata, observed.UpdatedState)
	}
	blockResults := collectBlockResults(blockSteps)

	reportDraftStep, err := steps.DispatchReportDraft(ctx, meta, observed.UpdatedState, memoryStep.Result.Retrieved, observed.Evidence, planStep.Plan, blockResults, observed.Diff, nil)
	if err != nil {
		return PortfolioRebalanceRunResult{}, handleAgentFailure(workflowRuntime, execCtx, runtimepkg.WorkflowStateActing, err, "portfolio rebalance report draft failed")
	}
	report, err := portfolioRebalanceReportFromPayload(reportDraftStep.Draft)
	if err != nil {
		return PortfolioRebalanceRunResult{}, err
	}
	meta = updateCausation(meta, reportDraftStep.Metadata.ResponseMetadata, observed.UpdatedState)

	verificationAssembler := contextview.VerificationContextAssembler{}
	blockVerificationContexts := make([]contextview.BlockVerificationContext, 0, len(blockSteps))
	for _, blockStep := range blockSteps {
		verificationContext, err := verificationAssembler.AssembleBlock(
			blockContextSpec(planStep.Plan, blockStep.Block),
			blockStep.Result,
			observed.UpdatedState,
			memoryStep.Result.Retrieved,
			observed.Evidence,
		)
		if err != nil {
			return PortfolioRebalanceRunResult{}, err
		}
		blockVerificationContexts = append(blockVerificationContexts, verificationContext)
	}
	finalVerificationContext, err := verificationAssembler.AssembleFinal(
		planStep.Plan.PlanID,
		reportDraftStep.Draft.Summary(),
		observed.UpdatedState,
		memoryStep.Result.Retrieved,
		observed.Evidence,
	)
	if err != nil {
		return PortfolioRebalanceRunResult{}, err
	}

	verificationStep, err := steps.DispatchVerification(ctx, meta, agents.VerificationStepInput{
		CurrentState:              observed.UpdatedState,
		Evidence:                  observed.Evidence,
		Memories:                  memoryStep.Result.Retrieved,
		Plan:                      planStep.Plan,
		BlockResults:              blockResults,
		BlockVerificationContexts: blockVerificationContexts,
		FinalVerificationContext:  finalVerificationContext,
		Report:                    reportDraftStep.Draft,
	})
	if err != nil {
		return PortfolioRebalanceRunResult{}, handleAgentFailure(workflowRuntime, execCtx, runtimepkg.WorkflowStateVerifying, err, "portfolio rebalance verification failed")
	}
	meta = updateCausation(meta, verificationStep.Metadata.ResponseMetadata, observed.UpdatedState)
	if verification.NeedsReplan(verificationStep.Result.Results) {
		nextState, _, err := workflowRuntime.HandleFailure(execCtx, runtimepkg.WorkflowStateVerifying, runtimepkg.FailureCategoryValidation, "portfolio rebalance verification failed; workflow should replan")
		if err != nil {
			return PortfolioRebalanceRunResult{}, err
		}
		return PortfolioRebalanceRunResult{
			WorkflowID:     workflowID,
			TaskSpec:       spec,
			Plan:           planStep.Plan,
			Evidence:       observed.Evidence,
			BlockResults:   blockResults,
			UpdatedState:   observed.UpdatedState,
			Report:         report,
			CoverageReport: verificationStep.Result.CoverageReport,
			Verification:   verificationStep.Result.Results,
			Oracle:         verificationStep.Result.OracleVerdict,
			RuntimeState:   nextState,
		}, nil
	}

	governanceStep, err := steps.DispatchGovernance(ctx, meta, observed.UpdatedState, reportDraftStep.Draft, nil)
	if err != nil {
		return PortfolioRebalanceRunResult{}, handleAgentFailure(workflowRuntime, execCtx, runtimepkg.WorkflowStateVerifying, err, "portfolio rebalance governance failed")
	}

	var (
		artifacts        []WorkflowArtifact
		approvalDecision *governance.PolicyDecision
		approvalAudit    *governance.AuditEvent
		checkpoint       *runtimepkg.CheckpointRecord
		resumeToken      *runtimepkg.ResumeToken
		pendingApproval  *runtimepkg.HumanApprovalPending
		runtimeState     = runtimepkg.WorkflowStateCompleted
	)
	if governanceStep.Approval.Decision != nil {
		approvalDecision = governanceStep.Approval.Decision
	}
	if governanceStep.Approval.Audit != nil {
		approvalAudit = governanceStep.Approval.Audit
	}

	switch {
	case approvalDecision != nil && approvalDecision.Outcome == governance.PolicyDecisionRequireApproval:
		pending := runtimepkg.HumanApprovalPending{
			ApprovalID:      workflowID + "-approval",
			WorkflowID:      workflowID,
			RequestedAction: "portfolio_rebalance_report",
			RequiredRoles:   approvalRoles(governanceStep.Approval),
			RequestedAt:     now,
		}
		cp, token, err := workflowRuntime.Checkpoint(execCtx, runtimepkg.WorkflowStateVerifying, runtimepkg.WorkflowStateVerifying, observed.UpdatedState.Version.Sequence, "portfolio rebalance waiting approval")
		if err != nil {
			return PortfolioRebalanceRunResult{}, err
		}
		nextState, err := workflowRuntime.PauseForApproval(execCtx, runtimepkg.WorkflowStateVerifying, pending)
		if err != nil {
			return PortfolioRebalanceRunResult{}, err
		}
		report.ApprovalRequired = true
		checkpoint = &cp
		resumeToken = &token
		pendingApproval = &pending
		runtimeState = nextState
	case approvalDecision != nil && approvalDecision.Outcome == governance.PolicyDecisionDeny:
		nextState, _, err := workflowRuntime.HandleFailure(execCtx, runtimepkg.WorkflowStateVerifying, runtimepkg.FailureCategoryUnrecoverable, "governance denied portfolio rebalance publication")
		if err != nil {
			return PortfolioRebalanceRunResult{}, err
		}
		runtimeState = nextState
	case governanceStep.Disclosure.Decision.Outcome == governance.PolicyDecisionAllow || governanceStep.Disclosure.Decision.Outcome == governance.PolicyDecisionRedact:
		finalizeStep, err := steps.DispatchReportFinalize(ctx, meta, reportDraftStep.Draft, governanceStep.Disclosure.Decision)
		if err != nil {
			return PortfolioRebalanceRunResult{}, handleAgentFailure(workflowRuntime, execCtx, runtimepkg.WorkflowStateVerifying, err, "portfolio rebalance report finalization failed")
		}
		finalReport, err := portfolioRebalanceReportFromPayload(finalizeStep.Report)
		if err != nil {
			return PortfolioRebalanceRunResult{}, err
		}
		report = finalReport
		artifacts = finalizeStep.Artifacts
	default:
		runtimeState = runtimepkg.WorkflowStateFailed
	}

	return PortfolioRebalanceRunResult{
		WorkflowID:         workflowID,
		TaskSpec:           spec,
		Plan:               planStep.Plan,
		Evidence:           observed.Evidence,
		BlockResults:       blockResults,
		UpdatedState:       observed.UpdatedState,
		DraftPayload:       reportDraftStep.Draft,
		DisclosureDecision: governanceStep.Disclosure.Decision,
		Report:             report,
		Artifacts:          artifacts,
		CoverageReport:     verificationStep.Result.CoverageReport,
		Verification:       verificationStep.Result.Results,
		Oracle:             verificationStep.Result.OracleVerdict,
		RiskAssessment:     governanceStep.Approval.RiskAssessment,
		ApprovalDecision:   approvalDecision,
		ApprovalAudit:      approvalAudit,
		Checkpoint:         checkpoint,
		ResumeToken:        resumeToken,
		PendingApproval:    pendingApproval,
		RuntimeState:       runtimeState,
	}, nil
}

func (w PortfolioRebalanceWorkflow) systemSteps() agents.SystemStepBus {
	return w.SystemSteps
}

func (w PortfolioRebalanceWorkflow) now() time.Time {
	if w.Now != nil {
		return w.Now().UTC()
	}
	return time.Now().UTC()
}

type PortfolioRebalanceWorkflowCapability struct {
	Workflow PortfolioRebalanceWorkflow
}

func (c PortfolioRebalanceWorkflowCapability) CapabilityName() string {
	return "portfolio_rebalance_workflow"
}

func (c PortfolioRebalanceWorkflowCapability) Execute(
	ctx context.Context,
	spec taskspec.TaskSpec,
	activation runtimepkg.FollowUpActivationContext,
	current state.FinancialWorldState,
) (runtimepkg.FollowUpWorkflowRunResult, error) {
	result, err := c.Workflow.RunTask(ctx, spec, activation, current)
	if err != nil {
		return runtimepkg.FollowUpWorkflowRunResult{}, err
	}
	return runtimepkg.FollowUpWorkflowRunResult{
		WorkflowID:           result.WorkflowID,
		RuntimeState:         result.RuntimeState,
		UpdatedState:         result.UpdatedState,
		Artifacts:            result.Artifacts,
		Checkpoint:           result.Checkpoint,
		ResumeToken:          result.ResumeToken,
		CheckpointPayload:    buildFollowUpFinalizeResumePayload(activation.ParentGraphID, result.WorkflowID, spec.ID, reporting.ArtifactKindPortfolioRebalanceReport, result.DraftPayload, result.DisclosureDecision, result.UpdatedState, result.RuntimeState),
		PendingApproval:      result.PendingApproval,
		FailureCategory:      followUpFailureCategoryFromRuntimeState(result.RuntimeState, runtimepkg.FailureCategoryValidation, runtimepkg.FailureCategoryUnrecoverable),
		FailureSummary:       followUpFailureSummary(result.RuntimeState, "portfolio rebalance follow-up did not complete"),
		LastRecoveryStrategy: followUpRecoveryStrategyFromRuntimeState(result.RuntimeState),
	}, nil
}

func (c PortfolioRebalanceWorkflowCapability) Resume(
	ctx context.Context,
	spec taskspec.TaskSpec,
	activation runtimepkg.FollowUpActivationContext,
	current state.FinancialWorldState,
	checkpoint runtimepkg.CheckpointRecord,
	token runtimepkg.ResumeToken,
	payload runtimepkg.CheckpointPayloadEnvelope,
) (runtimepkg.FollowUpWorkflowRunResult, error) {
	finalized, artifacts, err := resumeFollowUpFinalize(
		ctx,
		c.Workflow.systemSteps(),
		runtimepkg.ResolveWorkflowRuntime(c.Workflow.Runtime, checkpoint.WorkflowID, c.Workflow.Now),
		checkpoint.WorkflowID,
		"portfolio_rebalance_workflow_resume",
		spec,
		activation,
		current,
		checkpoint,
		token,
		payload,
	)
	if err != nil {
		return runtimepkg.FollowUpWorkflowRunResult{}, err
	}
	if _, err := portfolioRebalanceReportFromPayload(finalized); err != nil {
		return runtimepkg.FollowUpWorkflowRunResult{}, err
	}
	return runtimepkg.FollowUpWorkflowRunResult{
		WorkflowID:           checkpoint.WorkflowID,
		RuntimeState:         runtimepkg.WorkflowStateCompleted,
		UpdatedState:         current,
		Artifacts:            artifacts,
		FailureCategory:      "",
		FailureSummary:       "",
		LastRecoveryStrategy: runtimepkg.RecoveryStrategyWaitForApproval,
	}, nil
}
