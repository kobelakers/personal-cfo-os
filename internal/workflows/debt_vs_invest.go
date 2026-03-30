package workflows

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/agents"
	contextview "github.com/kobelakers/personal-cfo-os/internal/context"
	"github.com/kobelakers/personal-cfo-os/internal/governance"
	"github.com/kobelakers/personal-cfo-os/internal/reporting"
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

	memoryStep, err := steps.DispatchMemorySync(ctx, meta, observed.UpdatedState, observed.Evidence, "")
	if err != nil {
		return DebtDecisionRunResult{}, handleAgentFailure(workflowRuntime, execCtx, runtimepkg.WorkflowStateActing, err, "memory steward failed")
	}
	meta = updateCausation(meta, memoryStep.Metadata.ResponseMetadata, observed.UpdatedState)

	planStep, err := steps.DispatchPlan(ctx, meta, observed.UpdatedState, memoryStep.Result.Retrieved, observed.Evidence)
	if err != nil {
		return DebtDecisionRunResult{}, handleAgentFailure(workflowRuntime, execCtx, runtimepkg.WorkflowStatePlanning, err, "planner agent failed")
	}
	meta = updateCausation(meta, planStep.Metadata.ResponseMetadata, observed.UpdatedState)

	executionAssembler := contextview.ExecutionContextAssembler{}
	blockSteps := make([]agents.AnalysisBlockStepResult, 0, len(planStep.Plan.Blocks))
	for _, block := range planStep.Plan.Blocks {
		executionContext, err := executionAssembler.Assemble(blockContextSpec(planStep.Plan, block), observed.UpdatedState, memoryStep.Result.Retrieved, observed.Evidence)
		if err != nil {
			return DebtDecisionRunResult{}, err
		}
		blockStep, err := steps.DispatchAnalysisBlock(ctx, meta, block, observed.UpdatedState, memoryStep.Result.Retrieved, observed.Evidence, executionContext)
		if err != nil {
			return DebtDecisionRunResult{}, handleAgentFailure(workflowRuntime, execCtx, runtimepkg.WorkflowStateActing, err, "domain analysis block failed")
		}
		blockSteps = append(blockSteps, blockStep)
		meta = updateCausation(meta, blockStep.Metadata.ResponseMetadata, observed.UpdatedState)
	}
	blockResults := collectBlockResults(blockSteps)

	reportDraftStep, err := steps.DispatchReportDraft(ctx, meta, observed.UpdatedState, memoryStep.Result.Retrieved, observed.Evidence, planStep.Plan, blockResults, observed.Diff, nil)
	if err != nil {
		return DebtDecisionRunResult{}, handleAgentFailure(workflowRuntime, execCtx, runtimepkg.WorkflowStateActing, err, "report draft generation failed")
	}
	report, err := debtDecisionReportFromPayload(reportDraftStep.Draft)
	if err != nil {
		return DebtDecisionRunResult{}, err
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
			return DebtDecisionRunResult{}, err
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
		return DebtDecisionRunResult{}, err
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
		return DebtDecisionRunResult{}, handleAgentFailure(workflowRuntime, execCtx, runtimepkg.WorkflowStateVerifying, err, "verification agent failed")
	}
	meta = updateCausation(meta, verificationStep.Metadata.ResponseMetadata, observed.UpdatedState)

	runtimeState := runtimepkg.WorkflowStateCompleted
	if verification.HasTrustFailure(verificationStep.Result.Results) {
		nextState, _, err := workflowRuntime.HandleFailure(execCtx, runtimepkg.WorkflowStateVerifying, runtimepkg.FailureCategoryTrustValidation, "trust validation failed for debt decision report")
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
			BlockResults:   blockResults,
			UpdatedState:   observed.UpdatedState,
			DraftPayload:   reportDraftStep.Draft,
			Report:         report,
			CoverageReport: verificationStep.Result.CoverageReport,
			Verification:   verificationStep.Result.Results,
			Oracle:         verificationStep.Result.OracleVerdict,
			RuntimeState:   runtimeState,
		}, nil
	}
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
			BlockResults:   blockResults,
			UpdatedState:   observed.UpdatedState,
			DraftPayload:   reportDraftStep.Draft,
			Report:         report,
			CoverageReport: verificationStep.Result.CoverageReport,
			Verification:   verificationStep.Result.Results,
			Oracle:         verificationStep.Result.OracleVerdict,
			RuntimeState:   runtimeState,
		}, nil
	}

	governanceStep, err := steps.DispatchGovernance(ctx, meta, observed.UpdatedState, reportDraftStep.Draft, nil)
	if err != nil {
		return DebtDecisionRunResult{}, handleAgentFailure(workflowRuntime, execCtx, runtimepkg.WorkflowStateVerifying, err, "governance agent failed")
	}
	meta = updateCausation(meta, governanceStep.Metadata.ResponseMetadata, observed.UpdatedState)

	var artifacts []WorkflowArtifact
	var approvalDecision *governance.PolicyDecision
	var approvalAudit *governance.AuditEvent
	var checkpoint *runtimepkg.CheckpointRecord
	var resumeToken *runtimepkg.ResumeToken
	var pendingApproval *runtimepkg.HumanApprovalPending
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
			RequestedAction: "debt_vs_invest_recommendation",
			RequiredRoles:   approvalRoles(governanceStep.Approval),
			RequestedAt:     now,
		}
		cp, token, err := workflowRuntime.Checkpoint(execCtx, runtimepkg.WorkflowStateVerifying, runtimepkg.WorkflowStateVerifying, observed.UpdatedState.Version.Sequence, "debt decision waiting approval")
		if err != nil {
			return DebtDecisionRunResult{}, err
		}
		nextState, err := workflowRuntime.PauseForApproval(execCtx, runtimepkg.WorkflowStateVerifying, pending)
		if err != nil {
			return DebtDecisionRunResult{}, err
		}
		runtimeState = nextState
		report.ApprovalRequired = true
		checkpoint = &cp
		resumeToken = &token
		pendingApproval = &pending
	case approvalDecision != nil && approvalDecision.Outcome == governance.PolicyDecisionDeny:
		nextState, _, err := workflowRuntime.HandleFailure(execCtx, runtimepkg.WorkflowStateVerifying, runtimepkg.FailureCategoryGovernanceDenied, "governance denied debt decision publication")
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
		WorkflowID:         workflowID,
		Intake:             intake,
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

func (w DebtVsInvestWorkflow) systemSteps() agents.SystemStepBus {
	return w.SystemSteps
}

func (w DebtVsInvestWorkflow) now() time.Time {
	if w.Now != nil {
		return w.Now().UTC()
	}
	return time.Now().UTC()
}

func (w DebtVsInvestWorkflow) ResumeAfterApproval(
	ctx context.Context,
	spec taskspec.TaskSpec,
	activation runtimepkg.FollowUpActivationContext,
	current state.FinancialWorldState,
	checkpoint runtimepkg.CheckpointRecord,
	token runtimepkg.ResumeToken,
	draft reporting.ReportPayload,
	disclosure governance.PolicyDecision,
) (DebtDecisionRunResult, error) {
	finalized, artifacts, err := resumeFollowUpFinalize(
		ctx,
		w.systemSteps(),
		runtimepkg.ResolveWorkflowRuntime(w.Runtime, checkpoint.WorkflowID, w.Now),
		checkpoint.WorkflowID,
		"debt_vs_invest_workflow_resume",
		spec,
		activation,
		current,
		checkpoint,
		token,
		runtimepkg.CheckpointPayloadEnvelope{
			Kind: runtimepkg.CheckpointPayloadKindFollowUpFinalizeResume,
			FollowUpFinalizeResume: &runtimepkg.FollowUpFinalizeResumePayload{
				GraphID:                 firstNonEmptyString(activation.ParentGraphID, checkpoint.WorkflowID),
				TaskID:                  spec.ID,
				WorkflowID:              checkpoint.WorkflowID,
				ArtifactKind:            reporting.ArtifactKindDebtDecisionReport,
				DraftReport:             draft,
				DisclosureDecision:      disclosure,
				PendingStateSnapshotRef: firstNonEmptyString(current.Version.SnapshotID, checkpoint.ID),
			},
		},
	)
	if err != nil {
		return DebtDecisionRunResult{}, err
	}
	report, err := debtDecisionReportFromPayload(finalized)
	if err != nil {
		return DebtDecisionRunResult{}, err
	}
	return DebtDecisionRunResult{
		WorkflowID:         checkpoint.WorkflowID,
		TaskSpec:           spec,
		UpdatedState:       current,
		DraftPayload:       draft,
		DisclosureDecision: disclosure,
		Report:             report,
		Artifacts:          artifacts,
		RuntimeState:       runtimepkg.WorkflowStateCompleted,
	}, nil
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
