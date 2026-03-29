package workflows

import (
	"testing"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/governance"
	"github.com/kobelakers/personal-cfo-os/internal/memory"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
	runtimepkg "github.com/kobelakers/personal-cfo-os/internal/runtime"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
	"github.com/kobelakers/personal-cfo-os/internal/verification"
)

func TestTaxOptimizationWorkflowHappyPath(t *testing.T) {
	deps := buildPhase2Deps(t, safeHoldingsCSV, false, true)
	event := sampleSalaryChangeEvent(deps.Now)
	deadline := sampleSalaryDeadline(deps.Now, event)
	workflow := buildTaxOptimizationWorkflow(t, deps, []observation.LifeEventRecord{event}, []observation.CalendarDeadlineRecord{deadline})
	workflow.SystemSteps = buildSystemStepBusWithApprovalMinRisk(t, deps, governance.MemoryWritePolicy{
		MinConfidence:   0.7,
		RequireEvidence: false,
		AllowKinds: []memory.MemoryKind{
			memory.MemoryKindEpisodic,
			memory.MemoryKindSemantic,
			memory.MemoryKindProcedural,
		},
	}, governance.ActionRiskCritical)
	current := parentObservedStateForLifeEvent(t, deps, event, []observation.CalendarDeadlineRecord{deadline})

	result, err := workflow.RunTask(t.Context(), sampleTaxOptimizationTaskSpec(deps.Now), sampleFollowUpActivationContext(event, deadline, 1), current)
	if err != nil {
		t.Fatalf("run tax optimization workflow: %v", err)
	}
	if result.RuntimeState != runtimepkg.WorkflowStateCompleted {
		t.Fatalf("expected completed runtime state, got %q with verification=%+v", result.RuntimeState, result.Verification)
	}
	if len(result.BlockResults) != 1 || result.BlockResults[0].Tax == nil {
		t.Fatalf("expected single tax block result, got %+v", result.BlockResults)
	}
	if result.Report.Summary == "" || len(result.Report.RiskFlags) == 0 {
		t.Fatalf("expected populated tax report with risk flags, got %+v", result.Report)
	}
	if len(result.Artifacts) != 1 || result.Artifacts[0].Kind != ArtifactKindTaxOptimizationReport {
		t.Fatalf("expected finalized tax optimization artifact, got %+v", result.Artifacts)
	}
	if hasVerificationStatus(result.Verification, verification.VerificationStatusFail) || hasVerificationStatus(result.Verification, verification.VerificationStatusNeedsReplan) {
		t.Fatalf("expected tax optimization verification to pass, got %+v", result.Verification)
	}
}

func TestTaxOptimizationWorkflowMissingDeadlineTriggersReplan(t *testing.T) {
	deps := buildPhase2Deps(t, safeHoldingsCSV, false, true)
	event := sampleSalaryChangeEvent(deps.Now)
	workflow := buildTaxOptimizationWorkflow(t, deps, []observation.LifeEventRecord{event}, nil)
	spec := sampleTaxOptimizationTaskSpec(deps.Now)
	spec.RequiredEvidence = append(spec.RequiredEvidence, taskspec.RequiredEvidenceRef{
		Type:      "calendar_deadline",
		Reason:    "tax optimization must cover an active follow-up deadline",
		Mandatory: true,
	})

	result, err := workflow.RunTask(t.Context(), spec, sampleFollowUpActivationContext(event, observation.CalendarDeadlineRecord{}, 1), stateZero())
	if err != nil {
		t.Fatalf("run tax optimization workflow without deadline: %v", err)
	}
	if result.RuntimeState != runtimepkg.WorkflowStateReplanning {
		t.Fatalf("expected replanning runtime state, got %q", result.RuntimeState)
	}
	if !hasVerificationStatus(result.Verification, verification.VerificationStatusNeedsReplan) {
		t.Fatalf("expected needs_replan verification result, got %+v", result.Verification)
	}
}

func TestTaxOptimizationWorkflowFailsFastWithoutActivationSeed(t *testing.T) {
	deps := buildPhase2Deps(t, safeHoldingsCSV, false, true)
	event := sampleSalaryChangeEvent(deps.Now)
	deadline := sampleSalaryDeadline(deps.Now, event)
	workflow := buildTaxOptimizationWorkflow(t, deps, []observation.LifeEventRecord{event}, []observation.CalendarDeadlineRecord{deadline})

	_, err := workflow.RunTask(t.Context(), sampleTaxOptimizationTaskSpec(deps.Now), runtimepkg.FollowUpActivationContext{}, stateZero())
	if err == nil {
		t.Fatalf("expected follow-up workflow to fail fast without activation seed")
	}
}

func TestTaxOptimizationWorkflowApprovalPath(t *testing.T) {
	deps := buildPhase2Deps(t, safeHoldingsCSV, false, true)
	event := sampleSalaryChangeEvent(deps.Now)
	deadline := sampleSalaryDeadline(deps.Now, event)
	workflow := buildTaxOptimizationWorkflow(t, deps, []observation.LifeEventRecord{event}, []observation.CalendarDeadlineRecord{deadline})
	current := stateZero()
	current.RiskState.OverallRisk = "high"
	current.Version = state.StateVersion{Sequence: 1, SnapshotID: "state-v1", UpdatedAt: deps.Now}

	result, err := workflow.RunTask(t.Context(), sampleTaxOptimizationTaskSpec(deps.Now), sampleFollowUpActivationContext(event, deadline, 1), current)
	if err != nil {
		t.Fatalf("run approval-gated tax optimization workflow: %v", err)
	}
	if result.ApprovalDecision == nil || result.ApprovalDecision.Outcome != governance.PolicyDecisionRequireApproval {
		t.Fatalf("expected approval requirement, got %+v", result.ApprovalDecision)
	}
	if result.RuntimeState != runtimepkg.WorkflowStateWaitingApproval {
		t.Fatalf("expected waiting approval runtime state, got %q", result.RuntimeState)
	}
	if result.PendingApproval == nil || result.Checkpoint == nil || result.ResumeToken == nil {
		t.Fatalf("expected resumability anchors for waiting approval, got checkpoint=%+v token=%+v pending=%+v", result.Checkpoint, result.ResumeToken, result.PendingApproval)
	}
}

func TestPortfolioRebalanceWorkflowHappyPath(t *testing.T) {
	deps := buildPhase2Deps(t, safeHoldingsCSV, true, true)
	event := sampleJobChangeEvent(deps.Now)
	deadline := sampleJobDeadline(deps.Now, event)
	workflow := buildPortfolioRebalanceWorkflow(t, deps, []observation.LifeEventRecord{event}, []observation.CalendarDeadlineRecord{deadline})
	workflow.SystemSteps = buildSystemStepBusWithApprovalMinRisk(t, deps, governance.MemoryWritePolicy{
		MinConfidence:   0.7,
		RequireEvidence: false,
		AllowKinds: []memory.MemoryKind{
			memory.MemoryKindEpisodic,
			memory.MemoryKindSemantic,
			memory.MemoryKindProcedural,
		},
	}, governance.ActionRiskCritical)
	current := parentObservedStateForLifeEvent(t, deps, event, []observation.CalendarDeadlineRecord{deadline})

	result, err := workflow.RunTask(t.Context(), samplePortfolioRebalanceTaskSpec(deps.Now), sampleFollowUpActivationContext(event, deadline, 1), current)
	if err != nil {
		t.Fatalf("run portfolio rebalance workflow: %v", err)
	}
	if result.RuntimeState != runtimepkg.WorkflowStateCompleted {
		t.Fatalf("expected completed runtime state, got %q with verification=%+v", result.RuntimeState, result.Verification)
	}
	if len(result.BlockResults) != 1 || result.BlockResults[0].Portfolio == nil {
		t.Fatalf("expected single portfolio block result, got %+v", result.BlockResults)
	}
	if result.Report.Summary == "" || len(result.Report.RiskFlags) == 0 {
		t.Fatalf("expected populated portfolio report with risk flags, got %+v", result.Report)
	}
	if len(result.Artifacts) != 1 || result.Artifacts[0].Kind != ArtifactKindPortfolioRebalanceReport {
		t.Fatalf("expected finalized portfolio rebalance artifact, got %+v", result.Artifacts)
	}
	if hasVerificationStatus(result.Verification, verification.VerificationStatusFail) || hasVerificationStatus(result.Verification, verification.VerificationStatusNeedsReplan) {
		t.Fatalf("expected portfolio rebalance verification to pass, got %+v", result.Verification)
	}
}

func TestPortfolioRebalanceWorkflowMissingPortfolioSnapshotTriggersReplan(t *testing.T) {
	deps := buildPhase2Deps(t, "user_id,account_id,snapshot_at,asset_class,symbol,market_value_cents,target_allocation\n", true, true)
	event := sampleJobChangeEvent(deps.Now)
	deadline := sampleJobDeadline(deps.Now, event)
	workflow := buildPortfolioRebalanceWorkflow(t, deps, []observation.LifeEventRecord{event}, []observation.CalendarDeadlineRecord{deadline})
	spec := samplePortfolioRebalanceTaskSpec(deps.Now)
	spec.RequiredEvidence = append(spec.RequiredEvidence, taskspec.RequiredEvidenceRef{
		Type:      "portfolio_allocation_snapshot",
		Reason:    "portfolio rebalance must observe the latest allocation baseline",
		Mandatory: true,
	})

	result, err := workflow.RunTask(t.Context(), spec, sampleFollowUpActivationContext(event, deadline, 1), stateZero())
	if err != nil {
		t.Fatalf("run portfolio rebalance workflow without holdings baseline: %v", err)
	}
	if result.RuntimeState != runtimepkg.WorkflowStateReplanning {
		t.Fatalf("expected replanning runtime state, got %q", result.RuntimeState)
	}
	if !hasVerificationStatus(result.Verification, verification.VerificationStatusNeedsReplan) {
		t.Fatalf("expected needs_replan verification result, got %+v", result.Verification)
	}
}

func TestPortfolioRebalanceWorkflowApprovalPath(t *testing.T) {
	deps := buildPhase2Deps(t, string(readWorkflowFixture(t, "holdings_2026-03.csv")), true, true)
	event := sampleJobChangeEvent(deps.Now)
	deadline := sampleJobDeadline(deps.Now, event)
	workflow := buildPortfolioRebalanceWorkflow(t, deps, []observation.LifeEventRecord{event}, []observation.CalendarDeadlineRecord{deadline})
	current := stateZero()
	current.RiskState.OverallRisk = "high"
	current.Version = state.StateVersion{Sequence: 1, SnapshotID: "state-v1", UpdatedAt: deps.Now}

	result, err := workflow.RunTask(t.Context(), samplePortfolioRebalanceTaskSpec(deps.Now), sampleFollowUpActivationContext(event, deadline, 1), current)
	if err != nil {
		t.Fatalf("run approval-gated portfolio rebalance workflow: %v", err)
	}
	if result.ApprovalDecision == nil || result.ApprovalDecision.Outcome != governance.PolicyDecisionRequireApproval {
		t.Fatalf("expected approval requirement, got %+v", result.ApprovalDecision)
	}
	if result.RuntimeState != runtimepkg.WorkflowStateWaitingApproval {
		t.Fatalf("expected waiting approval runtime state, got %q", result.RuntimeState)
	}
	if result.PendingApproval == nil || result.Checkpoint == nil || result.ResumeToken == nil {
		t.Fatalf("expected resumability anchors for waiting approval, got checkpoint=%+v token=%+v pending=%+v", result.Checkpoint, result.ResumeToken, result.PendingApproval)
	}
}

func sampleTaxOptimizationTaskSpec(now time.Time) taskspec.TaskSpec {
	return taskspec.TaskSpec{
		ID:    "task-tax-optimization-test",
		Goal:  "复核税务优化、预扣调整和税优账户动作。",
		Scope: taskspec.TaskScope{Areas: []string{"tax", "cashflow"}, Notes: []string{"generated_follow_up"}},
		Constraints: taskspec.ConstraintSet{
			Hard: []string{"must remain evidence-backed"},
		},
		RiskLevel: taskspec.RiskLevelMedium,
		SuccessCriteria: []taskspec.SuccessCriteria{
			{ID: "tax-follow-up-grounded", Description: "tax optimization remains grounded in event and deadline evidence"},
		},
		RequiredEvidence: []taskspec.RequiredEvidenceRef{
			{Type: "event_signal", Reason: "stay tied to the triggering life event", Mandatory: true},
			{Type: "calendar_deadline", Reason: "capture deadline-sensitive tax actions", Mandatory: false},
			{Type: "tax_document", Reason: "reflect tax-document signals", Mandatory: false},
			{Type: "payslip_statement", Reason: "reflect payroll withholding context", Mandatory: false},
		},
		ApprovalRequirement: taskspec.ApprovalRequirementRecommended,
		UserIntentType:      taskspec.UserIntentTaxOptimization,
		CreatedAt:           now,
	}
}

func samplePortfolioRebalanceTaskSpec(now time.Time) taskspec.TaskSpec {
	return taskspec.TaskSpec{
		ID:    "task-portfolio-rebalance-test",
		Goal:  "复核组合再平衡窗口、配置漂移与流动性缓冲。",
		Scope: taskspec.TaskScope{Areas: []string{"portfolio", "cashflow"}, Notes: []string{"generated_follow_up"}},
		Constraints: taskspec.ConstraintSet{
			Hard: []string{"must remain evidence-backed"},
		},
		RiskLevel: taskspec.RiskLevelMedium,
		SuccessCriteria: []taskspec.SuccessCriteria{
			{ID: "portfolio-follow-up-grounded", Description: "portfolio rebalance remains grounded in event and portfolio evidence"},
		},
		RequiredEvidence: []taskspec.RequiredEvidenceRef{
			{Type: "event_signal", Reason: "stay tied to the triggering life event", Mandatory: true},
			{Type: "portfolio_allocation_snapshot", Reason: "reflect allocation baseline", Mandatory: true},
			{Type: "transaction_batch", Reason: "reflect contribution and liquidity changes", Mandatory: false},
		},
		ApprovalRequirement: taskspec.ApprovalRequirementRecommended,
		UserIntentType:      taskspec.UserIntentPortfolioRebalance,
		CreatedAt:           now,
	}
}

func sampleFollowUpActivationContext(event observation.LifeEventRecord, deadline observation.CalendarDeadlineRecord, depth int) runtimepkg.FollowUpActivationContext {
	sourceEvidenceIDs := []string{"evidence-life-event-" + event.ID}
	if deadline.ID != "" {
		sourceEvidenceIDs = append(sourceEvidenceIDs, "evidence-calendar-deadline-"+deadline.ID)
	}
	return runtimepkg.FollowUpActivationContext{
		GraphID:           "graph-follow-up-test",
		ParentGraphID:     "graph-follow-up-test",
		ParentWorkflowID:  "workflow-life-event-test",
		RootTaskID:        "task-life-event-test",
		TriggeredByTaskID: "task-generated-follow-up-test",
		RootCorrelationID: "workflow-life-event-test",
		TriggerSource:     taskspec.TaskTriggerSourceLifeEvent,
		GenerationReasons: []taskspec.TaskGenerationReason{
			{
				Code:             "life_event_impact",
				Description:      "generated from validated life event analysis",
				LifeEventID:      event.ID,
				LifeEventKind:    string(event.Kind),
				EvidenceIDs:      []string{"evidence-life-event-" + event.ID},
				DeadlineEvidence: sourceEvidenceIDs[1:],
			},
		},
		LifeEventID:       event.ID,
		LifeEventKind:     string(event.Kind),
		SourceEvidenceIDs: sourceEvidenceIDs,
		ExecutionDepth:    depth,
	}
}

func parentObservedStateForLifeEvent(
	t *testing.T,
	deps phase2Deps,
	event observation.LifeEventRecord,
	deadlines []observation.CalendarDeadlineRecord,
) state.FinancialWorldState {
	t.Helper()
	workflow := buildLifeEventWorkflow(t, deps, []observation.LifeEventRecord{event}, deadlines)
	intake := taskspec.EventTriggeredIntakeService{Now: func() time.Time { return deps.Now }}.Build(event)
	if !intake.Accepted || intake.TaskSpec == nil {
		t.Fatalf("expected accepted life event intake, got %+v", intake)
	}
	observed, err := workflow.TriggerService.ObserveAndReduce(t.Context(), *intake.TaskSpec, event, "workflow-life-event-parent-observation", stateZero())
	if err != nil {
		t.Fatalf("observe parent life event state: %v", err)
	}
	return observed.UpdatedState
}

func sampleSalaryChangeEvent(now time.Time) observation.LifeEventRecord {
	return observation.LifeEventRecord{
		ID:         "event-salary-follow-up",
		UserID:     "user-1",
		Kind:       observation.LifeEventSalaryChange,
		Source:     "hris",
		Provenance: "salary follow-up fixture",
		ObservedAt: now,
		Confidence: 0.95,
		SalaryChange: &observation.SalaryChangeEventPayload{
			PreviousMonthlyIncomeCents: 1000000,
			NewMonthlyIncomeCents:      1250000,
			EffectiveAt:                now.AddDate(0, 0, -1),
		},
	}
}

func sampleSalaryDeadline(now time.Time, event observation.LifeEventRecord) observation.CalendarDeadlineRecord {
	return observation.CalendarDeadlineRecord{
		ID:               "deadline-salary-follow-up",
		UserID:           "user-1",
		Kind:             "withholding_review",
		RelatedEventID:   event.ID,
		RelatedEventKind: event.Kind,
		Source:           "calendar",
		Provenance:       "salary deadline fixture",
		ObservedAt:       now,
		DeadlineAt:       now.Add(14 * 24 * time.Hour),
		Description:      "Review payroll withholding after salary change",
		Confidence:       0.9,
	}
}

func sampleJobChangeEvent(now time.Time) observation.LifeEventRecord {
	return observation.LifeEventRecord{
		ID:         "event-job-follow-up",
		UserID:     "user-1",
		Kind:       observation.LifeEventJobChange,
		Source:     "hris",
		Provenance: "job follow-up fixture",
		ObservedAt: now,
		Confidence: 0.94,
		JobChange: &observation.JobChangeEventPayload{
			PreviousEmployer:             "OldCo",
			NewEmployer:                  "NextCo",
			PreviousMonthlyIncomeCents:   1000000,
			NewMonthlyIncomeCents:        1400000,
			BenefitsEnrollmentDeadlineAt: now.Add(7 * 24 * time.Hour),
		},
	}
}

func sampleJobDeadline(now time.Time, event observation.LifeEventRecord) observation.CalendarDeadlineRecord {
	return observation.CalendarDeadlineRecord{
		ID:               "deadline-job-follow-up",
		UserID:           "user-1",
		Kind:             "benefits_enrollment",
		RelatedEventID:   event.ID,
		RelatedEventKind: event.Kind,
		Source:           "calendar",
		Provenance:       "job deadline fixture",
		ObservedAt:       now,
		DeadlineAt:       now.Add(7 * 24 * time.Hour),
		Description:      "Complete benefits enrollment after job change",
		Confidence:       0.9,
	}
}
