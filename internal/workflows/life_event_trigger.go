package workflows

import (
	"context"
	"fmt"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/agents"
	contextview "github.com/kobelakers/personal-cfo-os/internal/context"
	"github.com/kobelakers/personal-cfo-os/internal/governance"
	"github.com/kobelakers/personal-cfo-os/internal/observability"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
	runtimepkg "github.com/kobelakers/personal-cfo-os/internal/runtime"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
	"github.com/kobelakers/personal-cfo-os/internal/verification"
)

type LifeEventTriggerWorkflow struct {
	Intake         taskspec.EventTriggeredIntakeService
	TriggerService LifeEventWorkflowService
	SystemSteps    agents.SystemStepBus
	Runtime        runtimepkg.WorkflowRuntime
	EventLog       *observability.EventLog
	Now            func() time.Time
}

func (w LifeEventTriggerWorkflow) Run(
	ctx context.Context,
	event observation.LifeEventRecord,
	current state.FinancialWorldState,
) (LifeEventTriggerRunResult, error) {
	intake := w.Intake.Build(event)
	if !intake.Accepted || intake.TaskSpec == nil {
		return LifeEventTriggerRunResult{Intake: intake}, fmt.Errorf("life event intake rejected: %s", intake.FailureReason)
	}
	spec := *intake.TaskSpec
	now := w.now()
	if current.UserID == "" {
		current.UserID = event.UserID
	}

	workflowID := "workflow-life-event-" + now.Format("20060102150405")
	execCtx := runtimepkg.ExecutionContext{
		WorkflowID:    workflowID,
		TaskID:        spec.ID,
		CorrelationID: workflowID,
		Attempt:       1,
	}
	workflowRuntime := runtimepkg.ResolveWorkflowRuntime(w.Runtime, workflowID, w.Now)

	observed, err := w.TriggerService.ObserveAndReduce(ctx, spec, event, workflowID, current)
	if err != nil {
		return LifeEventTriggerRunResult{}, err
	}
	if _, _, err = workflowRuntime.Checkpoint(execCtx, runtimepkg.WorkflowStatePlanning, runtimepkg.WorkflowStatePlanning, current.Version.Sequence, "life event evidence observed"); err != nil {
		return LifeEventTriggerRunResult{}, err
	}
	if _, _, err = workflowRuntime.Checkpoint(execCtx, runtimepkg.WorkflowStateActing, runtimepkg.WorkflowStateActing, observed.Diff.ToVersion, "life event state updated from evidence patch"); err != nil {
		return LifeEventTriggerRunResult{}, err
	}

	steps := w.systemSteps()
	if steps == nil {
		return LifeEventTriggerRunResult{}, fmt.Errorf("life event workflow requires system step bus")
	}
	meta := systemStepMeta(workflowID, "life_event_trigger_workflow", spec, observed.UpdatedState, workflowID, workflowID)

	memoryStep, err := steps.DispatchMemorySync(ctx, meta, observed.UpdatedState, observed.Evidence, "")
	if err != nil {
		return LifeEventTriggerRunResult{}, handleAgentFailure(workflowRuntime, execCtx, runtimepkg.WorkflowStateActing, err, "memory steward failed")
	}
	meta = updateCausation(meta, memoryStep.Metadata.ResponseMetadata, observed.UpdatedState)

	planStep, err := steps.DispatchPlan(ctx, meta, observed.UpdatedState, memoryStep.Result.Retrieved, observed.Evidence)
	if err != nil {
		return LifeEventTriggerRunResult{}, handleAgentFailure(workflowRuntime, execCtx, runtimepkg.WorkflowStatePlanning, err, "planner agent failed")
	}
	meta = updateCausation(meta, planStep.Metadata.ResponseMetadata, observed.UpdatedState)

	executionAssembler := contextview.ExecutionContextAssembler{}
	blockSteps := make([]agents.AnalysisBlockStepResult, 0, len(planStep.Plan.Blocks))
	for _, block := range planStep.Plan.Blocks {
		executionContext, err := executionAssembler.Assemble(blockContextSpec(planStep.Plan, block), observed.UpdatedState, memoryStep.Result.Retrieved, observed.Evidence)
		if err != nil {
			return LifeEventTriggerRunResult{}, err
		}
		blockStep, err := steps.DispatchAnalysisBlock(ctx, meta, block, observed.UpdatedState, memoryStep.Result.Retrieved, observed.Evidence, executionContext)
		if err != nil {
			return LifeEventTriggerRunResult{}, handleAgentFailure(workflowRuntime, execCtx, runtimepkg.WorkflowStateActing, err, "life event domain analysis failed")
		}
		blockSteps = append(blockSteps, blockStep)
		meta = updateCausation(meta, blockStep.Metadata.ResponseMetadata, observed.UpdatedState)
	}
	blockResults := collectBlockResults(blockSteps)

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
			return LifeEventTriggerRunResult{}, err
		}
		blockVerificationContexts = append(blockVerificationContexts, verificationContext)
	}

	verificationPass1, err := steps.DispatchVerification(ctx, meta, agents.VerificationStepInput{
		Stage:                     verification.VerificationStageAnalysisBlocks,
		CurrentState:              observed.UpdatedState,
		Evidence:                  observed.Evidence,
		Memories:                  memoryStep.Result.Retrieved,
		Plan:                      planStep.Plan,
		BlockResults:              blockResults,
		BlockVerificationContexts: blockVerificationContexts,
	})
	if err != nil {
		return LifeEventTriggerRunResult{}, handleAgentFailure(workflowRuntime, execCtx, runtimepkg.WorkflowStateVerifying, err, "life event analysis verification failed")
	}
	meta = updateCausation(meta, verificationPass1.Metadata.ResponseMetadata, observed.UpdatedState)
	if verification.NeedsReplan(verificationPass1.Result.Results) {
		nextState, _, err := workflowRuntime.HandleFailure(execCtx, runtimepkg.WorkflowStateVerifying, runtimepkg.FailureCategoryValidation, "life event analysis verification failed; workflow should replan")
		if err != nil {
			return LifeEventTriggerRunResult{}, err
		}
		return LifeEventTriggerRunResult{
			WorkflowID:           workflowID,
			Intake:               intake,
			TaskSpec:             spec,
			Plan:                 planStep.Plan,
			EventEvidence:        observed.EventEvidence,
			DeadlineEvidence:     observed.DeadlineEvidence,
			Evidence:             observed.Evidence,
			BlockResults:         blockResults,
			UpdatedState:         observed.UpdatedState,
			StateDiff:            observed.Diff,
			GeneratedMemories:    memoryStep.Result.GeneratedIDs,
			CoverageReport:       verificationPass1.Result.CoverageReport,
			AnalysisVerification: verificationPass1.Result.Results,
			Oracle:               verificationPass1.Result.OracleVerdict,
			RuntimeState:         nextState,
		}, nil
	}

	taskGenerationStep, err := steps.DispatchTaskGeneration(ctx, meta, observed.UpdatedState, append(append([]observation.EvidenceRecord{}, observed.EventEvidence...), observed.DeadlineEvidence...), memoryStep.Result.Retrieved, observed.Diff, planStep.Plan, blockResults)
	if err != nil {
		return LifeEventTriggerRunResult{}, handleAgentFailure(workflowRuntime, execCtx, runtimepkg.WorkflowStateActing, err, "task generation agent failed")
	}
	meta = updateCausation(meta, taskGenerationStep.Metadata.ResponseMetadata, observed.UpdatedState)
	appendWorkflowLog(w.EventLog, workflowID, "generated_task_graph", "generated follow-up task graph from validated life event analysis", map[string]string{
		"graph_id":           taskGenerationStep.TaskGraph.GraphID,
		"generated_task_ids": taskIDs(taskGenerationStep.TaskGraph),
		"generated_intents":  taskIntents(taskGenerationStep.TaskGraph),
		"suppression_notes":  joinStrings(taskGenerationStep.TaskGraph.SuppressionNotes),
		"generation_summary": taskGenerationStep.TaskGraph.GenerationSummary,
		"trigger_source":     string(taskGenerationStep.TaskGraph.TriggerSource),
	}, now)

	reportDraftStep, err := steps.DispatchReportDraft(ctx, meta, observed.UpdatedState, memoryStep.Result.Retrieved, observed.Evidence, planStep.Plan, blockResults, observed.Diff, &taskGenerationStep.TaskGraph)
	if err != nil {
		return LifeEventTriggerRunResult{}, handleAgentFailure(workflowRuntime, execCtx, runtimepkg.WorkflowStateActing, err, "life event assessment draft failed")
	}
	meta = updateCausation(meta, reportDraftStep.Metadata.ResponseMetadata, observed.UpdatedState)
	finalVerificationContext, err := verificationAssembler.AssembleFinal(
		planStep.Plan.PlanID,
		reportDraftStep.Draft.Summary(),
		observed.UpdatedState,
		memoryStep.Result.Retrieved,
		observed.Evidence,
	)
	if err != nil {
		return LifeEventTriggerRunResult{}, err
	}

	verificationPass2, err := steps.DispatchVerification(ctx, meta, agents.VerificationStepInput{
		Stage:                    verification.VerificationStageGeneratedTasksAndFinal,
		CurrentState:             observed.UpdatedState,
		Evidence:                 observed.Evidence,
		Memories:                 memoryStep.Result.Retrieved,
		Plan:                     planStep.Plan,
		BlockResults:             blockResults,
		FinalVerificationContext: finalVerificationContext,
		TaskGraph:                &taskGenerationStep.TaskGraph,
		Report:                   reportDraftStep.Draft,
	})
	if err != nil {
		return LifeEventTriggerRunResult{}, handleAgentFailure(workflowRuntime, execCtx, runtimepkg.WorkflowStateVerifying, err, "life event final verification failed")
	}
	meta = updateCausation(meta, verificationPass2.Metadata.ResponseMetadata, observed.UpdatedState)
	if verification.NeedsReplan(verificationPass2.Result.Results) {
		nextState, _, err := workflowRuntime.HandleFailure(execCtx, runtimepkg.WorkflowStateVerifying, runtimepkg.FailureCategoryValidation, "life event generated tasks or final assessment failed verification")
		if err != nil {
			return LifeEventTriggerRunResult{}, err
		}
		report, _ := lifeEventAssessmentFromPayload(reportDraftStep.Draft)
		return LifeEventTriggerRunResult{
			WorkflowID:           workflowID,
			Intake:               intake,
			TaskSpec:             spec,
			Plan:                 planStep.Plan,
			EventEvidence:        observed.EventEvidence,
			DeadlineEvidence:     observed.DeadlineEvidence,
			Evidence:             observed.Evidence,
			BlockResults:         blockResults,
			UpdatedState:         observed.UpdatedState,
			StateDiff:            observed.Diff,
			GeneratedMemories:    memoryStep.Result.GeneratedIDs,
			TaskGraph:            taskGenerationStep.TaskGraph,
			Report:               report,
			CoverageReport:       verificationPass2.Result.CoverageReport,
			AnalysisVerification: verificationPass1.Result.Results,
			FinalVerification:    verificationPass2.Result.Results,
			Oracle:               verificationPass2.Result.OracleVerdict,
			RuntimeState:         nextState,
		}, nil
	}

	governanceStep, err := steps.DispatchGovernance(ctx, meta, observed.UpdatedState, reportDraftStep.Draft, &taskGenerationStep.TaskGraph)
	if err != nil {
		return LifeEventTriggerRunResult{}, handleAgentFailure(workflowRuntime, execCtx, runtimepkg.WorkflowStateVerifying, err, "life event governance failed")
	}
	meta = updateCausation(meta, governanceStep.Metadata.ResponseMetadata, observed.UpdatedState)

	var (
		runtimeState       = runtimepkg.WorkflowStateCompleted
		approvalDecision   *governance.PolicyDecision
		approvalAudit      *governance.AuditEvent
		registration       runtimepkg.FollowUpRegistrationResult
		followUpExecution  runtimepkg.FollowUpExecutionBatchResult
		artifacts          []WorkflowArtifact
		reportPayload      = reportDraftStep.Draft
		lifeEventTaskGraph = applyApprovalDecisionToTaskGraph(taskGenerationStep.TaskGraph, governanceStep.Approval)
	)
	if governanceStep.Approval.Decision != nil {
		approvalDecision = governanceStep.Approval.Decision
	}
	if governanceStep.Approval.Audit != nil {
		approvalAudit = governanceStep.Approval.Audit
	}

	switch {
	case approvalDecision != nil && approvalDecision.Outcome == governance.PolicyDecisionDeny:
		nextState, _, err := workflowRuntime.HandleFailure(execCtx, runtimepkg.WorkflowStateVerifying, runtimepkg.FailureCategoryPolicy, "governance denied life event follow-up registration")
		if err != nil {
			return LifeEventTriggerRunResult{}, err
		}
		runtimeState = nextState
	case governanceStep.Disclosure.Decision.Outcome == governance.PolicyDecisionAllow || governanceStep.Disclosure.Decision.Outcome == governance.PolicyDecisionRedact:
		registration, err = workflowRuntime.RegisterFollowUpTasks(execCtx, lifeEventTaskGraph, observed.UpdatedState)
		if err != nil {
			return LifeEventTriggerRunResult{}, err
		}
		appendWorkflowLog(w.EventLog, workflowID, "life_event_follow_up_registration", "registered generated follow-up tasks after governance", map[string]string{
			"graph_id":            registration.Graph.GraphID,
			"registered_task_ids": taskIDs(registration.Graph),
			"generated_intents":   taskIntents(registration.Graph),
		}, now)
		reportPayload = applyFollowUpRegistrationToAssessment(reportPayload, registration)
		activation, err := workflowRuntime.ReevaluateTaskGraph(execCtx, lifeEventTaskGraph.GraphID)
		if err != nil {
			return LifeEventTriggerRunResult{}, err
		}
		appendWorkflowLog(w.EventLog, workflowID, "life_event_follow_up_activation", "reevaluated generated follow-up task graph before runtime execution", map[string]string{
			"graph_id":       activation.GraphID,
			"ready_task_ids": joinStrings(activation.ReadyTaskIDs),
		}, now)
		if approvalDecision != nil && approvalDecision.Outcome == governance.PolicyDecisionRequireApproval {
			nextState, err := workflowRuntime.PauseForApproval(execCtx, runtimepkg.WorkflowStateVerifying, runtimepkg.HumanApprovalPending{
				ApprovalID:      workflowID + "-approval",
				WorkflowID:      workflowID,
				RequestedAction: "life_event_follow_up_registration",
				RequiredRoles:   approvalRoles(governanceStep.Approval),
				RequestedAt:     now,
			})
			if err != nil {
				return LifeEventTriggerRunResult{}, err
			}
			runtimeState = nextState
		} else {
			followUpExecution, err = workflowRuntime.ExecuteReadyFollowUps(ctx, execCtx, lifeEventTaskGraph.GraphID, runtimepkg.DefaultAutoExecutionPolicy())
			if err != nil {
				return LifeEventTriggerRunResult{}, err
			}
			if len(followUpExecution.RegisteredTasks) > 0 {
				registration.RegisteredTasks = append([]runtimepkg.FollowUpTaskRecord{}, followUpExecution.RegisteredTasks...)
			}
			reportPayload = applyFollowUpExecutionToAssessment(reportPayload, followUpExecution)
			appendWorkflowLog(w.EventLog, workflowID, "life_event_follow_up_execution", "executed eligible follow-up tasks through runtime capability plane", map[string]string{
				"graph_id":             followUpExecution.GraphID,
				"executed_task_ids":    executedTaskIDs(followUpExecution.ExecutedTasks),
				"latest_state_version": fmt.Sprintf("%d", followUpExecution.LatestCommittedStateSnapshot.State.Version.Sequence),
			}, now)
			finalizeStep, err := steps.DispatchReportFinalize(ctx, meta, reportPayload, governanceStep.Disclosure.Decision)
			if err != nil {
				return LifeEventTriggerRunResult{}, handleAgentFailure(workflowRuntime, execCtx, runtimepkg.WorkflowStateVerifying, err, "life event assessment finalization failed")
			}
			reportPayload = finalizeStep.Report
			artifacts = finalizeStep.Artifacts
			appendWorkflowLog(w.EventLog, workflowID, "life_event_assessment_finalized", "finalized life event assessment secondary artifact", map[string]string{
				"artifact_count": fmt.Sprintf("%d", len(artifacts)),
			}, now)
		}
	default:
		runtimeState = runtimepkg.WorkflowStateFailed
	}

	report, err := lifeEventAssessmentFromPayload(reportPayload)
	if err != nil {
		return LifeEventTriggerRunResult{}, err
	}
	return LifeEventTriggerRunResult{
		WorkflowID:           workflowID,
		Intake:               intake,
		TaskSpec:             spec,
		Plan:                 planStep.Plan,
		EventEvidence:        observed.EventEvidence,
		DeadlineEvidence:     observed.DeadlineEvidence,
		Evidence:             observed.Evidence,
		BlockResults:         blockResults,
		UpdatedState:         observed.UpdatedState,
		StateDiff:            observed.Diff,
		GeneratedMemories:    memoryStep.Result.GeneratedIDs,
		TaskGraph:            lifeEventTaskGraph,
		FollowUpTasks:        registration,
		FollowUpExecution:    followUpExecution,
		Report:               report,
		Artifacts:            artifacts,
		CoverageReport:       verificationPass2.Result.CoverageReport,
		AnalysisVerification: verificationPass1.Result.Results,
		FinalVerification:    verificationPass2.Result.Results,
		Oracle:               verificationPass2.Result.OracleVerdict,
		RiskAssessment:       governanceStep.Approval.RiskAssessment,
		ApprovalDecision:     approvalDecision,
		ApprovalAudit:        approvalAudit,
		RuntimeState:         runtimeState,
	}, nil
}

func (w LifeEventTriggerWorkflow) systemSteps() agents.SystemStepBus {
	return w.SystemSteps
}

func (w LifeEventTriggerWorkflow) now() time.Time {
	if w.Now != nil {
		return w.Now().UTC()
	}
	return time.Now().UTC()
}

func applyApprovalDecisionToTaskGraph(
	graph taskspec.TaskGraph,
	approval governance.ApprovalEvaluation,
) taskspec.TaskGraph {
	if approval.Decision == nil || approval.Decision.Outcome != governance.PolicyDecisionRequireApproval {
		return graph
	}
	updated := graph
	updated.GeneratedTasks = make([]taskspec.GeneratedTaskSpec, 0, len(graph.GeneratedTasks))
	for _, item := range graph.GeneratedTasks {
		item.Metadata.RequiresApproval = true
		updated.GeneratedTasks = append(updated.GeneratedTasks, item)
	}
	return updated
}
