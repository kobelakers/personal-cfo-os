package workflows

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/agents"
	"github.com/kobelakers/personal-cfo-os/internal/analysis"
	contextview "github.com/kobelakers/personal-cfo-os/internal/context"
	"github.com/kobelakers/personal-cfo-os/internal/governance"
	"github.com/kobelakers/personal-cfo-os/internal/memory"
	"github.com/kobelakers/personal-cfo-os/internal/observability"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/planning"
	"github.com/kobelakers/personal-cfo-os/internal/reducers"
	"github.com/kobelakers/personal-cfo-os/internal/reporting"
	runtimepkg "github.com/kobelakers/personal-cfo-os/internal/runtime"
	"github.com/kobelakers/personal-cfo-os/internal/skills"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
	"github.com/kobelakers/personal-cfo-os/internal/tools"
	"github.com/kobelakers/personal-cfo-os/internal/verification"
)

type BehaviorInterventionObservationResult struct {
	Evidence     []observation.EvidenceRecord `json:"evidence"`
	UpdatedState state.FinancialWorldState    `json:"updated_state"`
	Diff         state.StateDiff              `json:"diff"`
}

type BehaviorInterventionService struct {
	QueryTransaction tools.QueryTransactionTool
	QueryLiability   tools.QueryLiabilityTool
	QueryPortfolio   tools.QueryPortfolioTool
	ReducerEngine    reducers.DeterministicReducerEngine
	StateReducer     state.DefaultStateReducer
}

func (s BehaviorInterventionService) ObserveAndReduce(
	ctx context.Context,
	spec taskspec.TaskSpec,
	userID string,
	workflowID string,
	current state.FinancialWorldState,
) (BehaviorInterventionObservationResult, error) {
	input := observationInput(spec, userID)
	transactionEvidence, err := s.QueryTransaction.QueryEvidence(ctx, input)
	if err != nil {
		return BehaviorInterventionObservationResult{}, err
	}
	liabilityEvidence, err := s.QueryLiability.QueryEvidence(ctx, input)
	if err != nil {
		return BehaviorInterventionObservationResult{}, err
	}
	portfolioEvidence, err := s.QueryPortfolio.QueryEvidence(ctx, input)
	if err != nil {
		return BehaviorInterventionObservationResult{}, err
	}
	evidence := dedupeEvidence(append(append(transactionEvidence, liabilityEvidence...), portfolioEvidence...))
	patch, err := s.ReducerEngine.BuildPatch(current, evidence, spec.ID, workflowID, "observed")
	if err != nil {
		return BehaviorInterventionObservationResult{}, err
	}
	updatedState, diff, err := s.StateReducer.ApplyEvidencePatch(current, patch)
	if err != nil {
		return BehaviorInterventionObservationResult{}, err
	}
	return BehaviorInterventionObservationResult{
		Evidence:     evidence,
		UpdatedState: updatedState,
		Diff:         diff,
	}, nil
}

type BehaviorInterventionWorkflow struct {
	Intake              taskspec.DeterministicIntakeService
	Service             BehaviorInterventionService
	SystemSteps         agents.SystemStepBus
	Runtime             runtimepkg.WorkflowRuntime
	MemoryService       memory.WorkflowMemoryService
	SkillSelector       skills.SkillSelector
	SkillRuntime        skills.SkillRuntime
	SkillExecutionStore runtimepkg.SkillExecutionStore
	EventLog            *observability.EventLog
	Now                 func() time.Time
}

func (w BehaviorInterventionWorkflow) Run(
	ctx context.Context,
	userID string,
	rawInput string,
	current state.FinancialWorldState,
) (BehaviorInterventionRunResult, error) {
	intake := w.Intake.Parse(rawInput)
	if !intake.Accepted || intake.TaskSpec == nil {
		return BehaviorInterventionRunResult{Intake: intake}, fmt.Errorf("task intake rejected: %s", intake.FailureReason)
	}
	spec := *intake.TaskSpec
	now := w.now()
	if current.UserID == "" {
		current.UserID = userID
	}

	workflowID := "workflow-behavior-intervention-" + now.Format("20060102150405")
	execCtx := runtimepkg.ExecutionContext{
		WorkflowID:    workflowID,
		TaskID:        spec.ID,
		CorrelationID: workflowID,
		Attempt:       1,
	}
	workflowRuntime := runtimepkg.ResolveWorkflowRuntime(w.Runtime, workflowID, w.Now)

	observed, err := w.Service.ObserveAndReduce(ctx, spec, userID, workflowID, current)
	if err != nil {
		return BehaviorInterventionRunResult{}, err
	}
	if _, _, err = workflowRuntime.Checkpoint(execCtx, runtimepkg.WorkflowStatePlanning, runtimepkg.WorkflowStatePlanning, current.Version.Sequence, "behavior evidence observed"); err != nil {
		return BehaviorInterventionRunResult{}, err
	}
	if _, _, err = workflowRuntime.Checkpoint(execCtx, runtimepkg.WorkflowStateActing, runtimepkg.WorkflowStateActing, observed.Diff.ToVersion, "behavior state updated from evidence patch"); err != nil {
		return BehaviorInterventionRunResult{}, err
	}

	steps := w.systemSteps()
	if steps == nil {
		return BehaviorInterventionRunResult{}, fmt.Errorf("behavior intervention workflow requires system step bus")
	}
	meta := systemStepMeta(workflowID, "behavior_intervention_workflow", spec, observed.UpdatedState, workflowID, workflowID)

	memoryStep, err := steps.DispatchMemorySync(ctx, meta, observed.UpdatedState, observed.Evidence, "")
	if err != nil {
		return BehaviorInterventionRunResult{}, handleAgentFailure(workflowRuntime, execCtx, runtimepkg.WorkflowStateActing, err, "behavior memory steward failed")
	}
	meta = updateCausation(meta, memoryStep.Metadata.ResponseMetadata, observed.UpdatedState)

	planStep, err := steps.DispatchPlan(ctx, meta, observed.UpdatedState, memoryStep.Result.Retrieved, observed.Evidence)
	if err != nil {
		return BehaviorInterventionRunResult{}, handleAgentFailure(workflowRuntime, execCtx, runtimepkg.WorkflowStatePlanning, err, "behavior planner failed")
	}
	meta = updateCausation(meta, planStep.Metadata.ResponseMetadata, observed.UpdatedState)
	if len(planStep.Plan.Blocks) != 1 {
		return BehaviorInterventionRunResult{}, fmt.Errorf("behavior intervention workflow expects exactly one block, got %d", len(planStep.Plan.Blocks))
	}

	selectedSkill, err := w.selectSkill(planStep.Plan.Blocks[0], spec, observed.UpdatedState, observed.Evidence, memoryStep.Result.BehaviorRetrieved)
	if err != nil {
		return BehaviorInterventionRunResult{}, err
	}
	planStep.Plan.Blocks[0].SelectedSkill = &selectedSkill
	w.logSkillSelection(workflowID, selectedSkill, memoryStep.Result.BehaviorRetrieved, observed.Evidence)

	executionAssembler := contextview.ExecutionContextAssembler{}
	blockSteps := make([]agents.AnalysisBlockStepResult, 0, len(planStep.Plan.Blocks))
	for i, block := range planStep.Plan.Blocks {
		executionContext, err := executionAssembler.Assemble(blockContextSpec(planStep.Plan, block), observed.UpdatedState, memoryStep.Result.Retrieved, observed.Evidence)
		if err != nil {
			return BehaviorInterventionRunResult{}, err
		}
		blockStep, err := steps.DispatchAnalysisBlock(ctx, meta, block, observed.UpdatedState, memoryStep.Result.Retrieved, observed.Evidence, executionContext)
		if err != nil {
			return BehaviorInterventionRunResult{}, handleAgentFailure(workflowRuntime, execCtx, runtimepkg.WorkflowStateActing, err, "behavior domain block failed")
		}
		blockSteps = append(blockSteps, blockStep)
		meta = updateCausation(meta, blockStep.Metadata.ResponseMetadata, observed.UpdatedState)
		planStep.Plan.Blocks[i].SelectedSkill = &selectedSkill
	}
	blockResults := collectBlockResults(blockSteps)
	result := blockResults[0].Behavior
	if result == nil {
		return BehaviorInterventionRunResult{}, fmt.Errorf("behavior workflow requires behavior block result")
	}
	w.logSkillExecution(workflowID, *result)

	reportDraftStep, err := steps.DispatchReportDraft(ctx, meta, observed.UpdatedState, memoryStep.Result.Retrieved, observed.Evidence, planStep.Plan, blockResults, observed.Diff, nil)
	if err != nil {
		return BehaviorInterventionRunResult{}, handleAgentFailure(workflowRuntime, execCtx, runtimepkg.WorkflowStateActing, err, "behavior report draft failed")
	}
	report, err := behaviorInterventionReportFromPayload(reportDraftStep.Draft)
	if err != nil {
		return BehaviorInterventionRunResult{}, err
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
			return BehaviorInterventionRunResult{}, err
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
		return BehaviorInterventionRunResult{}, err
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
		return BehaviorInterventionRunResult{}, handleAgentFailure(workflowRuntime, execCtx, runtimepkg.WorkflowStateVerifying, err, "behavior verification failed")
	}
	meta = updateCausation(meta, verificationStep.Metadata.ResponseMetadata, observed.UpdatedState)

	baseResult := BehaviorInterventionRunResult{
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
	}

	if verification.HasTrustFailure(verificationStep.Result.Results) {
		nextState, _, err := workflowRuntime.HandleFailure(execCtx, runtimepkg.WorkflowStateVerifying, runtimepkg.FailureCategoryTrustValidation, "behavior trust validation failed")
		if err != nil {
			return BehaviorInterventionRunResult{}, err
		}
		record, memoryRefs, persistErr := w.persistSkillOutcome(ctx, spec, workflowID, string(planStep.Plan.Blocks[0].ID), *result, nextState, "trust_validation_failed", "", nil)
		if persistErr != nil {
			return BehaviorInterventionRunResult{}, persistErr
		}
		baseResult.RuntimeState = nextState
		baseResult.GeneratedMemories = append(baseResult.GeneratedMemories, memoryRefs...)
		baseResult.SkillExecution = &record
		return baseResult, nil
	}
	if verification.NeedsReplan(verificationStep.Result.Results) {
		nextState, _, err := workflowRuntime.HandleFailure(execCtx, runtimepkg.WorkflowStateVerifying, runtimepkg.FailureCategoryValidation, "behavior verification failed; workflow should replan")
		if err != nil {
			return BehaviorInterventionRunResult{}, err
		}
		record, memoryRefs, persistErr := w.persistSkillOutcome(ctx, spec, workflowID, string(planStep.Plan.Blocks[0].ID), *result, nextState, "validation_failed", "", nil)
		if persistErr != nil {
			return BehaviorInterventionRunResult{}, persistErr
		}
		baseResult.RuntimeState = nextState
		baseResult.GeneratedMemories = append(baseResult.GeneratedMemories, memoryRefs...)
		baseResult.SkillExecution = &record
		return baseResult, nil
	}

	governanceStep, err := steps.DispatchGovernance(ctx, meta, observed.UpdatedState, reportDraftStep.Draft, nil)
	if err != nil {
		return BehaviorInterventionRunResult{}, handleAgentFailure(workflowRuntime, execCtx, runtimepkg.WorkflowStateVerifying, err, "behavior governance failed")
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
			RequestedAction: "behavior_intervention_report",
			RequiredRoles:   approvalRoles(governanceStep.Approval),
			RequestedAt:     now,
		}
		cp, token, err := workflowRuntime.Checkpoint(execCtx, runtimepkg.WorkflowStateVerifying, runtimepkg.WorkflowStateVerifying, observed.UpdatedState.Version.Sequence, "behavior intervention waiting approval")
		if err != nil {
			return BehaviorInterventionRunResult{}, err
		}
		nextState, err := workflowRuntime.PauseForApproval(execCtx, runtimepkg.WorkflowStateVerifying, pending)
		if err != nil {
			return BehaviorInterventionRunResult{}, err
		}
		report.ApprovalRequired = true
		checkpoint = &cp
		resumeToken = &token
		pendingApproval = &pending
		runtimeState = nextState
	case approvalDecision != nil && approvalDecision.Outcome == governance.PolicyDecisionDeny:
		nextState, _, err := workflowRuntime.HandleFailure(execCtx, runtimepkg.WorkflowStateVerifying, runtimepkg.FailureCategoryGovernanceDenied, "governance denied behavior intervention publication")
		if err != nil {
			return BehaviorInterventionRunResult{}, err
		}
		runtimeState = nextState
	case governanceStep.Disclosure.Decision.Outcome == governance.PolicyDecisionAllow || governanceStep.Disclosure.Decision.Outcome == governance.PolicyDecisionRedact:
		finalizeStep, err := steps.DispatchReportFinalize(ctx, meta, reportDraftStep.Draft, governanceStep.Disclosure.Decision)
		if err != nil {
			return BehaviorInterventionRunResult{}, handleAgentFailure(workflowRuntime, execCtx, runtimepkg.WorkflowStateVerifying, err, "behavior report finalization failed")
		}
		finalReport, err := behaviorInterventionReportFromPayload(finalizeStep.Report)
		if err != nil {
			return BehaviorInterventionRunResult{}, err
		}
		report = finalReport
		artifacts = finalizeStep.Artifacts
	default:
		runtimeState = runtimepkg.WorkflowStateFailed
	}

	governanceOutcome := ""
	if approvalDecision != nil {
		governanceOutcome = string(approvalDecision.Outcome)
	} else if governanceStep.Disclosure.Decision.Outcome != "" {
		governanceOutcome = string(governanceStep.Disclosure.Decision.Outcome)
	}
	artifactRefs := artifactIDs(artifacts)
	record, memoryRefs, err := w.persistSkillOutcome(ctx, spec, workflowID, string(planStep.Plan.Blocks[0].ID), *result, runtimeState, "", governanceOutcome, artifactRefs)
	if err != nil {
		return BehaviorInterventionRunResult{}, err
	}

	baseResult.DisclosureDecision = governanceStep.Disclosure.Decision
	baseResult.Report = report
	baseResult.Artifacts = artifacts
	baseResult.RiskAssessment = governanceStep.Approval.RiskAssessment
	baseResult.ApprovalDecision = approvalDecision
	baseResult.ApprovalAudit = approvalAudit
	baseResult.Checkpoint = checkpoint
	baseResult.ResumeToken = resumeToken
	baseResult.PendingApproval = pendingApproval
	baseResult.RuntimeState = runtimeState
	baseResult.GeneratedMemories = append(baseResult.GeneratedMemories, memoryStep.Result.GeneratedIDs...)
	baseResult.GeneratedMemories = append(baseResult.GeneratedMemories, memoryRefs...)
	baseResult.SkillExecution = &record
	return baseResult, nil
}

func (w BehaviorInterventionWorkflow) ResumeAfterApproval(
	ctx context.Context,
	spec taskspec.TaskSpec,
	activation runtimepkg.FollowUpActivationContext,
	current state.FinancialWorldState,
	checkpoint runtimepkg.CheckpointRecord,
	token runtimepkg.ResumeToken,
	draft reporting.ReportPayload,
	disclosure governance.PolicyDecision,
) (BehaviorInterventionRunResult, error) {
	finalized, artifacts, err := resumeFollowUpFinalize(
		ctx,
		w.systemSteps(),
		runtimepkg.ResolveWorkflowRuntime(w.Runtime, checkpoint.WorkflowID, w.Now),
		checkpoint.WorkflowID,
		"behavior_intervention_workflow_resume",
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
				ArtifactKind:            reporting.ArtifactKindBehaviorInterventionReport,
				DraftReport:             draft,
				DisclosureDecision:      disclosure,
				PendingStateSnapshotRef: firstNonEmptyString(current.Version.SnapshotID, checkpoint.ID),
			},
		},
	)
	if err != nil {
		return BehaviorInterventionRunResult{}, err
	}
	report, err := behaviorInterventionReportFromPayload(finalized)
	if err != nil {
		return BehaviorInterventionRunResult{}, err
	}
	result := BehaviorInterventionRunResult{
		WorkflowID:         checkpoint.WorkflowID,
		TaskSpec:           spec,
		UpdatedState:       current,
		DraftPayload:       draft,
		DisclosureDecision: disclosure,
		Report:             report,
		Artifacts:          artifacts,
		RuntimeState:       runtimepkg.WorkflowStateCompleted,
	}
	if draft.BehaviorIntervention != nil {
		selection := skills.SkillSelection{
			Family:   skills.SkillFamily(draft.BehaviorIntervention.SelectedSkillFamily),
			Version:  skills.SkillVersion(draft.BehaviorIntervention.SelectedSkillVersion),
			RecipeID: draft.BehaviorIntervention.SelectedRecipeID,
		}
		record := skills.SkillExecutionRecord{
			WorkflowID: checkpoint.WorkflowID,
			TaskID:     spec.ID,
			ExecutionID: behaviorSkillExecutionID(checkpoint.WorkflowID, "behavior_intervention_block"),
			Selection:  selection,
			Status:     skills.SkillExecutionStatusGoverned,
			GovernanceOutcome: "approved",
			ProducedArtifactIDs: artifactIDs(artifacts),
			CreatedAt:  w.now(),
			UpdatedAt:  w.now(),
		}
		if w.SkillExecutionStore != nil {
			if err := w.SkillExecutionStore.Save(record); err != nil {
				return BehaviorInterventionRunResult{}, err
			}
		}
		result.SkillExecution = &record
		outcome := memory.SkillOutcomeMemory{
			WorkflowID:            checkpoint.WorkflowID,
			TaskID:                spec.ID,
			TraceID:               checkpoint.WorkflowID,
			SkillFamily:           string(selection.Family),
			SkillVersion:          string(selection.Version),
			RecipeID:              selection.RecipeID,
			InterventionIntensity: selection.InterventionIntensity,
			FinalRuntimeState:     string(runtimepkg.WorkflowStateCompleted),
			GovernanceOutcome:     "approved",
			ApprovalOutcome:       "approved",
			ArtifactRefs:          artifactIDs(artifacts),
		}
		ids, err := w.MemoryService.WriteBehaviorSkillOutcome(ctx, spec, outcome)
		if err != nil {
			return BehaviorInterventionRunResult{}, err
		}
		result.GeneratedMemories = ids
		w.logSkillOutcomeMemory(checkpoint.WorkflowID, ids, outcome)
	}
	return result, nil
}

func (w BehaviorInterventionWorkflow) selectSkill(
	block planning.ExecutionBlock,
	spec taskspec.TaskSpec,
	current state.FinancialWorldState,
	evidence []observation.EvidenceRecord,
	retrieved []memory.MemoryRecord,
) (skills.SkillSelection, error) {
	if w.SkillSelector == nil {
		return skills.SkillSelection{}, fmt.Errorf("behavior workflow requires skill selector")
	}
	selection, err := w.SkillSelector.Select(skills.SelectionInput{
		Task:               spec,
		AllowedFamilies:    append([]string{}, block.AllowedSkillFamilies...),
		SelectionHint:      block.SkillSelectionHint,
		CurrentState:       current,
		Evidence:           evidence,
		ProceduralMemories: memory.SkillSelectionMemoryRecords(retrieved),
	})
	if err != nil {
		return skills.SkillSelection{}, err
	}
	if w.SkillRuntime != nil {
		if _, _, err := w.SkillRuntime.Resolve(selection); err != nil {
			return skills.SkillSelection{}, err
		}
	}
	return selection, nil
}

func (w BehaviorInterventionWorkflow) persistSkillOutcome(
	ctx context.Context,
	spec taskspec.TaskSpec,
	workflowID string,
	blockID string,
	result analysis.BehaviorBlockResult,
	runtimeState runtimepkg.WorkflowExecutionState,
	validatorOutcome string,
	governanceOutcome string,
	artifactRefs []string,
) (skills.SkillExecutionRecord, []string, error) {
	record := skills.SkillExecutionRecord{
		WorkflowID:         workflowID,
		TaskID:             spec.ID,
		ExecutionID:        behaviorSkillExecutionID(workflowID, blockID),
		Selection:          result.SelectedSkill,
		Status:             skillStatusForRuntime(runtimeState),
		ValidatorOutcome:   validatorOutcome,
		GovernanceOutcome:  governanceOutcome,
		ProducedArtifactIDs: append([]string{}, artifactRefs...),
		CreatedAt:          w.now(),
		UpdatedAt:          w.now(),
	}
	if w.SkillExecutionStore != nil {
		if err := w.SkillExecutionStore.Save(record); err != nil {
			return skills.SkillExecutionRecord{}, nil, err
		}
	}
	outcome := memory.SkillOutcomeMemory{
		WorkflowID:            workflowID,
		TaskID:                spec.ID,
		TraceID:               workflowID,
		SkillFamily:           string(result.SelectedSkill.Family),
		SkillVersion:          string(result.SelectedSkill.Version),
		RecipeID:              result.SelectedSkill.RecipeID,
		AnomalyCodes:          riskFlagCodes(result.RiskFlags),
		InterventionIntensity: result.SelectedSkill.InterventionIntensity,
		FinalRuntimeState:     string(runtimeState),
		GovernanceOutcome:     governanceOutcome,
		ApprovalOutcome:       approvalOutcomeForState(runtimeState, governanceOutcome),
		ArtifactRefs:          append([]string{}, artifactRefs...),
	}
	ids, err := w.MemoryService.WriteBehaviorSkillOutcome(ctx, spec, outcome)
	if err != nil {
		return skills.SkillExecutionRecord{}, nil, err
	}
	record.ProducedMemoryRefs = append([]string{}, ids...)
	if w.SkillExecutionStore != nil {
		record.UpdatedAt = w.now()
		if err := w.SkillExecutionStore.Save(record); err != nil {
			return skills.SkillExecutionRecord{}, nil, err
		}
	}
	w.logSkillOutcomeMemory(workflowID, ids, outcome)
	return record, ids, nil
}

func (w BehaviorInterventionWorkflow) systemSteps() agents.SystemStepBus {
	return w.SystemSteps
}

func (w BehaviorInterventionWorkflow) now() time.Time {
	if w.Now != nil {
		return w.Now().UTC()
	}
	return time.Now().UTC()
}

func (w BehaviorInterventionWorkflow) logSkillSelection(
	workflowID string,
	selection skills.SkillSelection,
	memories []memory.MemoryRecord,
	evidence []observation.EvidenceRecord,
) {
	reasons := make([]string, 0, len(selection.Reasons))
	for _, reason := range selection.Reasons {
		reasons = append(reasons, reason.Detail)
	}
	appendWorkflowLog(w.EventLog, workflowID, "skill_selected", fmt.Sprintf("selected %s/%s", selection.Family, selection.RecipeID), map[string]string{
		"skill_family":      string(selection.Family),
		"skill_version":     string(selection.Version),
		"recipe_id":         selection.RecipeID,
		"reason_count":      fmt.Sprintf("%d", len(selection.Reasons)),
		"reason_details":    joinStrings(reasons),
		"memory_refs":       joinStrings(selection.MemoryRefs),
		"evidence_refs":     joinStrings(selection.EvidenceRefs),
		"retrieved_memorys": joinStrings(memoryRecordIDs(memories)),
		"selected_evidence": joinEvidenceIDs(evidence),
	}, w.now())
}

func (w BehaviorInterventionWorkflow) logSkillExecution(workflowID string, result analysis.BehaviorBlockResult) {
	appendWorkflowLog(w.EventLog, workflowID, "skill_execution", result.Summary, map[string]string{
		"skill_family":            string(result.SelectedSkill.Family),
		"skill_version":           string(result.SelectedSkill.Version),
		"recipe_id":               result.SelectedSkill.RecipeID,
		"approval_required":       fmt.Sprintf("%t", result.ApprovalRequired),
		"skill_selection_reasons": joinStrings(result.SkillSelectionReasons),
		"policy_rule_refs":        joinStrings(result.PolicyRuleRefs),
	}, w.now())
}

func (w BehaviorInterventionWorkflow) logSkillOutcomeMemory(workflowID string, memoryIDs []string, outcome memory.SkillOutcomeMemory) {
	appendWorkflowLog(w.EventLog, workflowID, "skill_outcome_memory_written", fmt.Sprintf("wrote procedural memory for %s/%s", outcome.SkillFamily, outcome.RecipeID), map[string]string{
		"memory_ids":              joinStrings(memoryIDs),
		"skill_family":            outcome.SkillFamily,
		"skill_version":           outcome.SkillVersion,
		"recipe_id":               outcome.RecipeID,
		"final_runtime_state":     outcome.FinalRuntimeState,
		"governance_outcome":      outcome.GovernanceOutcome,
		"approval_outcome":        outcome.ApprovalOutcome,
		"intervention_intensity":  string(outcome.InterventionIntensity),
	}, w.now())
}

func behaviorSkillExecutionID(workflowID string, blockID string) string {
	return workflowID + ":skill:" + blockID
}

func behaviorInterventionReportFromPayload(payload reporting.ReportPayload) (BehaviorInterventionReport, error) {
	if payload.BehaviorIntervention == nil {
		return BehaviorInterventionReport{}, fmt.Errorf("behavior intervention report payload is missing")
	}
	return *payload.BehaviorIntervention, nil
}

func artifactIDs(items []WorkflowArtifact) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.ID) == "" {
			continue
		}
		result = append(result, item.ID)
	}
	return result
}

func memoryRecordIDs(items []memory.MemoryRecord) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.ID) == "" {
			continue
		}
		result = append(result, item.ID)
	}
	return result
}

func riskFlagCodes(items []analysis.RiskFlag) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.Code) == "" {
			continue
		}
		result = append(result, item.Code)
	}
	return result
}

func skillStatusForRuntime(runtimeState runtimepkg.WorkflowExecutionState) skills.SkillExecutionStatus {
	switch runtimeState {
	case runtimepkg.WorkflowStateFailed:
		return skills.SkillExecutionStatusFailed
	case runtimepkg.WorkflowStateWaitingApproval:
		return skills.SkillExecutionStatusGoverned
	default:
		return skills.SkillExecutionStatusExecuted
	}
}

func approvalOutcomeForState(runtimeState runtimepkg.WorkflowExecutionState, governanceOutcome string) string {
	switch runtimeState {
	case runtimepkg.WorkflowStateWaitingApproval:
		return "pending"
	case runtimepkg.WorkflowStateCompleted:
		if governanceOutcome == "approved" {
			return "approved"
		}
		return ""
	case runtimepkg.WorkflowStateFailed:
		if governanceOutcome == string(governance.PolicyDecisionDeny) {
			return "denied"
		}
		return "not_applicable"
	default:
		return ""
	}
}
