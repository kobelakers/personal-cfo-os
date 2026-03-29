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

type TaxOptimizationWorkflow struct {
	Service     FollowUpWorkflowService
	SystemSteps agents.SystemStepBus
	Runtime     runtimepkg.WorkflowRuntime
	EventLog    *observability.EventLog
	Now         func() time.Time
}

func (w TaxOptimizationWorkflow) RunTask(
	ctx context.Context,
	spec taskspec.TaskSpec,
	activation runtimepkg.FollowUpActivationContext,
	current state.FinancialWorldState,
) (TaxOptimizationRunResult, error) {
	if spec.UserIntentType != taskspec.UserIntentTaxOptimization {
		return TaxOptimizationRunResult{}, fmt.Errorf("tax optimization workflow requires tax_optimization task spec")
	}
	now := w.now()
	workflowID := "workflow-tax-optimization-" + now.Format("20060102150405")
	execCtx := runtimepkg.ExecutionContext{
		WorkflowID:    workflowID,
		TaskID:        spec.ID,
		CorrelationID: activation.RootCorrelationID,
		Attempt:       1,
	}
	workflowRuntime := runtimepkg.ResolveWorkflowRuntime(w.Runtime, workflowID, w.Now)
	observed, err := w.Service.ObserveAndReduce(ctx, spec, activation, workflowID, current)
	if err != nil {
		return TaxOptimizationRunResult{}, err
	}
	if _, _, err = workflowRuntime.Checkpoint(execCtx, runtimepkg.WorkflowStatePlanning, runtimepkg.WorkflowStatePlanning, current.Version.Sequence, "tax optimization evidence observed"); err != nil {
		return TaxOptimizationRunResult{}, err
	}
	if _, _, err = workflowRuntime.Checkpoint(execCtx, runtimepkg.WorkflowStateActing, runtimepkg.WorkflowStateActing, observed.Diff.ToVersion, "tax optimization state updated from evidence patch"); err != nil {
		return TaxOptimizationRunResult{}, err
	}

	steps := w.systemSteps()
	if steps == nil {
		return TaxOptimizationRunResult{}, fmt.Errorf("tax optimization workflow requires system step bus")
	}
	meta := systemStepMeta(workflowID, "tax_optimization_workflow", spec, observed.UpdatedState, activation.RootCorrelationID, activation.TriggeredByTaskID)

	memoryStep, err := steps.DispatchMemorySync(ctx, meta, observed.UpdatedState, observed.Evidence, "")
	if err != nil {
		return TaxOptimizationRunResult{}, handleAgentFailure(workflowRuntime, execCtx, runtimepkg.WorkflowStateActing, err, "tax optimization memory steward failed")
	}
	meta = updateCausation(meta, memoryStep.Metadata.ResponseMetadata, observed.UpdatedState)

	planStep, err := steps.DispatchPlan(ctx, meta, observed.UpdatedState, memoryStep.Result.Retrieved, observed.Evidence)
	if err != nil {
		return TaxOptimizationRunResult{}, handleAgentFailure(workflowRuntime, execCtx, runtimepkg.WorkflowStatePlanning, err, "tax optimization planner failed")
	}
	meta = updateCausation(meta, planStep.Metadata.ResponseMetadata, observed.UpdatedState)

	executionAssembler := contextview.ExecutionContextAssembler{}
	blockSteps := make([]agents.AnalysisBlockStepResult, 0, len(planStep.Plan.Blocks))
	for _, block := range planStep.Plan.Blocks {
		executionContext, err := executionAssembler.Assemble(blockContextSpec(planStep.Plan, block), observed.UpdatedState, memoryStep.Result.Retrieved, observed.Evidence)
		if err != nil {
			return TaxOptimizationRunResult{}, err
		}
		blockStep, err := steps.DispatchAnalysisBlock(ctx, meta, block, observed.UpdatedState, memoryStep.Result.Retrieved, observed.Evidence, executionContext)
		if err != nil {
			return TaxOptimizationRunResult{}, handleAgentFailure(workflowRuntime, execCtx, runtimepkg.WorkflowStateActing, err, "tax optimization domain block failed")
		}
		blockSteps = append(blockSteps, blockStep)
		meta = updateCausation(meta, blockStep.Metadata.ResponseMetadata, observed.UpdatedState)
	}
	blockResults := collectBlockResults(blockSteps)

	reportDraftStep, err := steps.DispatchReportDraft(ctx, meta, observed.UpdatedState, memoryStep.Result.Retrieved, observed.Evidence, planStep.Plan, blockResults, observed.Diff, nil)
	if err != nil {
		return TaxOptimizationRunResult{}, handleAgentFailure(workflowRuntime, execCtx, runtimepkg.WorkflowStateActing, err, "tax optimization report draft failed")
	}
	report, err := taxOptimizationReportFromPayload(reportDraftStep.Draft)
	if err != nil {
		return TaxOptimizationRunResult{}, err
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
			return TaxOptimizationRunResult{}, err
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
		return TaxOptimizationRunResult{}, err
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
		return TaxOptimizationRunResult{}, handleAgentFailure(workflowRuntime, execCtx, runtimepkg.WorkflowStateVerifying, err, "tax optimization verification failed")
	}
	meta = updateCausation(meta, verificationStep.Metadata.ResponseMetadata, observed.UpdatedState)
	if verification.NeedsReplan(verificationStep.Result.Results) {
		nextState, _, err := workflowRuntime.HandleFailure(execCtx, runtimepkg.WorkflowStateVerifying, runtimepkg.FailureCategoryValidation, "tax optimization verification failed; workflow should replan")
		if err != nil {
			return TaxOptimizationRunResult{}, err
		}
		return TaxOptimizationRunResult{
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
		return TaxOptimizationRunResult{}, handleAgentFailure(workflowRuntime, execCtx, runtimepkg.WorkflowStateVerifying, err, "tax optimization governance failed")
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
			RequestedAction: "tax_optimization_report",
			RequiredRoles:   approvalRoles(governanceStep.Approval),
			RequestedAt:     now,
		}
		cp, token, err := workflowRuntime.Checkpoint(execCtx, runtimepkg.WorkflowStateVerifying, runtimepkg.WorkflowStateVerifying, observed.UpdatedState.Version.Sequence, "tax optimization waiting approval")
		if err != nil {
			return TaxOptimizationRunResult{}, err
		}
		nextState, err := workflowRuntime.PauseForApproval(execCtx, runtimepkg.WorkflowStateVerifying, pending)
		if err != nil {
			return TaxOptimizationRunResult{}, err
		}
		report.ApprovalRequired = true
		checkpoint = &cp
		resumeToken = &token
		pendingApproval = &pending
		runtimeState = nextState
	case approvalDecision != nil && approvalDecision.Outcome == governance.PolicyDecisionDeny:
		nextState, _, err := workflowRuntime.HandleFailure(execCtx, runtimepkg.WorkflowStateVerifying, runtimepkg.FailureCategoryUnrecoverable, "governance denied tax optimization publication")
		if err != nil {
			return TaxOptimizationRunResult{}, err
		}
		runtimeState = nextState
	case governanceStep.Disclosure.Decision.Outcome == governance.PolicyDecisionAllow || governanceStep.Disclosure.Decision.Outcome == governance.PolicyDecisionRedact:
		finalizeStep, err := steps.DispatchReportFinalize(ctx, meta, reportDraftStep.Draft, governanceStep.Disclosure.Decision)
		if err != nil {
			return TaxOptimizationRunResult{}, handleAgentFailure(workflowRuntime, execCtx, runtimepkg.WorkflowStateVerifying, err, "tax optimization report finalization failed")
		}
		finalReport, err := taxOptimizationReportFromPayload(finalizeStep.Report)
		if err != nil {
			return TaxOptimizationRunResult{}, err
		}
		report = finalReport
		artifacts = finalizeStep.Artifacts
	default:
		runtimeState = runtimepkg.WorkflowStateFailed
	}

	return TaxOptimizationRunResult{
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

func (w TaxOptimizationWorkflow) systemSteps() agents.SystemStepBus {
	return w.SystemSteps
}

func (w TaxOptimizationWorkflow) now() time.Time {
	if w.Now != nil {
		return w.Now().UTC()
	}
	return time.Now().UTC()
}

type TaxOptimizationWorkflowCapability struct {
	Workflow TaxOptimizationWorkflow
}

func (c TaxOptimizationWorkflowCapability) CapabilityName() string {
	return "tax_optimization_workflow"
}

func (c TaxOptimizationWorkflowCapability) Execute(
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
		CheckpointPayload:    buildFollowUpFinalizeResumePayload(activation.ParentGraphID, result.WorkflowID, spec.ID, reporting.ArtifactKindTaxOptimizationReport, result.DraftPayload, result.DisclosureDecision, result.UpdatedState, result.RuntimeState),
		PendingApproval:      result.PendingApproval,
		FailureCategory:      followUpFailureCategoryFromRuntimeState(result.RuntimeState, runtimepkg.FailureCategoryValidation, runtimepkg.FailureCategoryUnrecoverable),
		FailureSummary:       followUpFailureSummary(result.RuntimeState, "tax optimization follow-up did not complete"),
		LastRecoveryStrategy: followUpRecoveryStrategyFromRuntimeState(result.RuntimeState),
	}, nil
}

func (c TaxOptimizationWorkflowCapability) Resume(
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
		"tax_optimization_workflow_resume",
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
	if _, err := taxOptimizationReportFromPayload(finalized); err != nil {
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
