package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kobelakers/personal-cfo-os/internal/governance"
	"github.com/kobelakers/personal-cfo-os/internal/observability"
	runtimepkg "github.com/kobelakers/personal-cfo-os/internal/runtime"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/verification"
	"github.com/kobelakers/personal-cfo-os/internal/workflows"
)

type Phase6BOptions = Phase5DOptions

type Phase6BEnvironment struct {
	*Phase5DEnvironment
}

type BehaviorIntervention6BRunOutput struct {
	Result workflows.BehaviorInterventionRunResult `json:"result"`
	Trace  observability.WorkflowTraceDump         `json:"trace"`
}

func OpenPhase6BEnvironment(options Phase6BOptions) (*Phase6BEnvironment, error) {
	base, err := OpenPhase5DEnvironment(Phase5DOptions(options))
	if err != nil {
		return nil, err
	}
	if base.BehaviorIntervention.SystemSteps == nil {
		_ = base.Close()
		return nil, fmt.Errorf("phase 6b environment requires behavior intervention workflow")
	}
	return &Phase6BEnvironment{Phase5DEnvironment: base}, nil
}

func (e *Phase6BEnvironment) RunBehaviorIntervention(
	ctx context.Context,
	userID string,
	rawInput string,
	current state.FinancialWorldState,
) (BehaviorIntervention6BRunOutput, error) {
	result, err := e.BehaviorIntervention.Run(ctx, userID, rawInput, current)
	if err != nil {
		return BehaviorIntervention6BRunOutput{}, err
	}
	trace := e.buildTrace(result.WorkflowID, result.Verification, result.Report.MetricRecords, result.ApprovalAudit)
	if err := e.persistBehaviorInterventionReplay(ctx, result, trace); err != nil {
		return BehaviorIntervention6BRunOutput{}, err
	}
	return BehaviorIntervention6BRunOutput{Result: result, Trace: trace}, nil
}

func (e *Phase6BEnvironment) ResumeBehaviorInterventionAfterApproval(
	ctx context.Context,
	result workflows.BehaviorInterventionRunResult,
) (BehaviorIntervention6BRunOutput, error) {
	if result.Checkpoint == nil || result.ResumeToken == nil {
		return BehaviorIntervention6BRunOutput{}, fmt.Errorf("behavior intervention result does not contain approval resume anchors")
	}
	resumed, err := e.BehaviorIntervention.ResumeAfterApproval(
		ctx,
		result.TaskSpec,
		runtimepkg.FollowUpActivationContext{
			RootCorrelationID: result.WorkflowID,
			ParentGraphID:     result.WorkflowID,
			TriggeredByTaskID: result.TaskSpec.ID,
		},
		result.UpdatedState,
		*result.Checkpoint,
		*result.ResumeToken,
		result.DraftPayload,
		result.DisclosureDecision,
	)
	if err != nil {
		return BehaviorIntervention6BRunOutput{}, err
	}
	trace := e.buildTrace(resumed.WorkflowID, resumed.Verification, resumed.Report.MetricRecords, resumed.ApprovalAudit)
	if err := e.persistBehaviorInterventionReplay(ctx, resumed, trace); err != nil {
		return BehaviorIntervention6BRunOutput{}, err
	}
	return BehaviorIntervention6BRunOutput{Result: resumed, Trace: trace}, nil
}

func (e *Phase6BEnvironment) persistBehaviorInterventionReplay(
	ctx context.Context,
	result workflows.BehaviorInterventionRunResult,
	trace observability.WorkflowTraceDump,
) error {
	approvalID := ""
	if result.PendingApproval != nil {
		approvalID = result.PendingApproval.ApprovalID
	} else if result.Report.ApprovalRequired {
		approvalID = result.WorkflowID + "-approval"
	}
	checkpointID := ""
	resumeToken := ""
	if result.Checkpoint != nil {
		checkpointID = result.Checkpoint.ID
	}
	if result.ResumeToken != nil {
		resumeToken = result.ResumeToken.Token
	}
	return e.persistWorkflowReplay(ctx, persistWorkflowReplayInput{
		WorkflowID:      result.WorkflowID,
		TaskID:          result.TaskSpec.ID,
		Intent:          string(result.TaskSpec.UserIntentType),
		RuntimeState:    result.RuntimeState,
		FailureCategory: behaviorInterventionFailureCategory(result),
		FailureSummary:  behaviorInterventionFailureSummary(result),
		ApprovalID:      approvalID,
		CheckpointID:    checkpointID,
		ResumeToken:     resumeToken,
		Summary:         result.Report.Summary,
		Artifacts:       result.Artifacts,
		Trace:           trace,
		Scenario:        "behavior_intervention_6b",
	})
}

func behaviorInterventionFailureCategory(result workflows.BehaviorInterventionRunResult) runtimepkg.FailureCategory {
	switch {
	case verification.HasTrustFailure(result.Verification):
		return runtimepkg.FailureCategoryTrustValidation
	case result.ApprovalDecision != nil && result.ApprovalDecision.Outcome == governance.PolicyDecisionDeny:
		return runtimepkg.FailureCategoryGovernanceDenied
	case result.RuntimeState == runtimepkg.WorkflowStateFailed:
		return runtimepkg.FailureCategoryValidation
	default:
		return ""
	}
}

func behaviorInterventionFailureSummary(result workflows.BehaviorInterventionRunResult) string {
	switch {
	case verification.HasTrustFailure(result.Verification):
		return "trust validation failed for behavior intervention report"
	case result.ApprovalDecision != nil && result.ApprovalDecision.Outcome == governance.PolicyDecisionDeny:
		return "governance denied behavior intervention publication"
	case result.RuntimeState == runtimepkg.WorkflowStateFailed:
		return "behavior intervention workflow failed before final completion"
	default:
		return ""
	}
}

func (o BehaviorIntervention6BRunOutput) WriteArtifact(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(o.Result.Report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, payload, 0o644)
}

func (o BehaviorIntervention6BRunOutput) WriteTrace(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(o.Trace, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, payload, 0o644)
}
