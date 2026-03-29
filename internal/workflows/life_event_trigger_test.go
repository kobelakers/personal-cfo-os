package workflows

import (
	"testing"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/observability"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
	runtimepkg "github.com/kobelakers/personal-cfo-os/internal/runtime"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
	"github.com/kobelakers/personal-cfo-os/internal/verification"
)

func TestLifeEventTriggerWorkflowExecutesCapabilityBackedFollowUps(t *testing.T) {
	deps := buildPhase2Deps(t, safeHoldingsCSV, false, true)
	event := observation.LifeEventRecord{
		ID:         "event-salary-1",
		UserID:     "user-1",
		Kind:       observation.LifeEventSalaryChange,
		Source:     "hris",
		Provenance: "salary fixture",
		ObservedAt: deps.Now,
		Confidence: 0.95,
		SalaryChange: &observation.SalaryChangeEventPayload{
			PreviousMonthlyIncomeCents: 1000000,
			NewMonthlyIncomeCents:      1250000,
			EffectiveAt:                deps.Now.AddDate(0, 0, -1),
		},
	}
	deadline := observation.CalendarDeadlineRecord{
		ID:               "deadline-salary-1",
		UserID:           "user-1",
		Kind:             "withholding_review",
		RelatedEventID:   event.ID,
		RelatedEventKind: event.Kind,
		Source:           "calendar",
		Provenance:       "deadline fixture",
		ObservedAt:       deps.Now,
		DeadlineAt:       deps.Now.Add(14 * 24 * time.Hour),
		Description:      "Review payroll withholding after salary change",
		Confidence:       0.9,
	}
	workflow := buildLifeEventWorkflow(t, deps, []observation.LifeEventRecord{event}, []observation.CalendarDeadlineRecord{deadline})

	result, err := workflow.Run(t.Context(), event, stateZero())
	if err != nil {
		t.Fatalf("run life event workflow: %v", err)
	}
	if result.RuntimeState != runtimepkg.WorkflowStateCompleted {
		t.Logf("analysis verification: %+v", result.AnalysisVerification)
		t.Logf("final verification: %+v", result.FinalVerification)
		t.Fatalf("expected completed runtime state, got %q", result.RuntimeState)
	}
	if len(result.BlockResults) != 3 {
		t.Fatalf("expected three domain block results, got %+v", result.BlockResults)
	}
	if len(result.TaskGraph.GeneratedTasks) != 2 {
		t.Fatalf("expected two generated follow-up tasks, got %+v", result.TaskGraph.GeneratedTasks)
	}
	if len(result.FollowUpExecution.ExecutedTasks) != 2 {
		t.Fatalf("expected two executed follow-up tasks, got %+v", result.FollowUpExecution.ExecutedTasks)
	}
	for _, record := range result.FollowUpExecution.ExecutedTasks {
		if record.RootCorrelationID != result.WorkflowID || record.ParentWorkflowID != result.WorkflowID || record.CausationID == "" {
			t.Fatalf("expected executed follow-up provenance chain to reference parent workflow and generated task, got %+v", record)
		}
	}
	if result.Report.EventSummary == "" || len(result.Report.GeneratedTaskIDs) != 2 {
		t.Fatalf("expected populated life event assessment report, got %+v", result.Report)
	}
	if len(result.Artifacts) != 1 || result.Artifacts[0].Kind != ArtifactKindLifeEventAssessment {
		t.Fatalf("expected finalized life event assessment artifact, got %+v", result.Artifacts)
	}

	taxTask := findFollowUpByIntent(t, result.FollowUpTasks.RegisteredTasks, taskspec.UserIntentTaxOptimization)
	if taxTask.Status != runtimepkg.TaskQueueStatusCompleted {
		t.Fatalf("expected tax follow-up to complete through capability-backed execution, got task=%+v execution=%+v", taxTask, result.FollowUpExecution.ExecutedTasks)
	}
	if taxTask.RequiredCapability == "" {
		t.Fatalf("expected tax follow-up to retain required capability metadata, got %+v", taxTask)
	}
	if status := result.Report.GeneratedTaskStatuses[taxTask.Task.ID]; status != string(runtimepkg.TaskQueueStatusCompleted) {
		t.Fatalf("expected report to capture completed tax follow-up status, got %q", status)
	}
	portfolioTask := findFollowUpByIntent(t, result.FollowUpTasks.RegisteredTasks, taskspec.UserIntentPortfolioRebalance)
	if portfolioTask.Status != runtimepkg.TaskQueueStatusCompleted {
		t.Fatalf("expected portfolio follow-up to complete through capability-backed execution, got task=%+v execution=%+v", portfolioTask, result.FollowUpExecution.ExecutedTasks)
	}
	if status := result.Report.GeneratedTaskStatuses[portfolioTask.Task.ID]; status != string(runtimepkg.TaskQueueStatusCompleted) {
		t.Fatalf("expected report to capture completed portfolio follow-up status, got %q", status)
	}
	if result.FollowUpExecution.LatestCommittedStateSnapshot.State.Version.Sequence <= result.UpdatedState.Version.Sequence {
		t.Fatalf("expected follow-up execution to advance committed state snapshot, got parent=%d latest=%d", result.UpdatedState.Version.Sequence, result.FollowUpExecution.LatestCommittedStateSnapshot.State.Version.Sequence)
	}
	if len(result.AnalysisVerification) == 0 || len(result.FinalVerification) == 0 {
		t.Fatalf("expected both verification passes to run")
	}
	if hasVerificationStatus(result.AnalysisVerification, verification.VerificationStatusFail) || hasVerificationStatus(result.FinalVerification, verification.VerificationStatusFail) {
		t.Fatalf("expected life event verification to pass, got analysis=%+v final=%+v", result.AnalysisVerification, result.FinalVerification)
	}
}

func TestLifeEventTriggerWorkflowApprovalPathRegistersWaitingTasks(t *testing.T) {
	deps := buildPhase2Deps(t, safeHoldingsCSV, true, true)
	event := observation.LifeEventRecord{
		ID:         "event-housing-1",
		UserID:     "user-1",
		Kind:       observation.LifeEventHousingChange,
		Source:     "housing",
		Provenance: "housing fixture",
		ObservedAt: deps.Now,
		Confidence: 0.94,
		HousingChange: &observation.HousingChangeEventPayload{
			PreviousMonthlyHousingCostCents: 200000,
			NewMonthlyHousingCostCents:      520000,
			MortgageBalanceCents:            26000000,
			EffectiveAt:                     deps.Now.AddDate(0, 0, -2),
		},
	}
	deadline := observation.CalendarDeadlineRecord{
		ID:               "deadline-housing-1",
		UserID:           "user-1",
		Kind:             "mortgage_review",
		RelatedEventID:   event.ID,
		RelatedEventKind: event.Kind,
		Source:           "calendar",
		Provenance:       "deadline fixture",
		ObservedAt:       deps.Now,
		DeadlineAt:       deps.Now.Add(10 * 24 * time.Hour),
		Description:      "Review mortgage and housing liquidity follow-up",
		Confidence:       0.9,
	}
	workflow := buildLifeEventWorkflow(t, deps, []observation.LifeEventRecord{event}, []observation.CalendarDeadlineRecord{deadline})

	result, err := workflow.Run(t.Context(), event, stateZero())
	if err != nil {
		t.Fatalf("run housing life event workflow: %v", err)
	}
	if result.RuntimeState != runtimepkg.WorkflowStateWaitingApproval {
		t.Logf("analysis verification: %+v", result.AnalysisVerification)
		t.Logf("final verification: %+v", result.FinalVerification)
		t.Fatalf("expected waiting approval runtime state, got %q", result.RuntimeState)
	}
	if len(result.Artifacts) != 0 {
		t.Fatalf("expected no finalized artifact before approval, got %+v", result.Artifacts)
	}
	debtTask := findFollowUpByIntent(t, result.FollowUpTasks.RegisteredTasks, taskspec.UserIntentDebtVsInvest)
	if debtTask.Status != runtimepkg.TaskQueueStatusWaitingApproval {
		t.Fatalf("expected debt-vs-invest follow-up to wait for approval, got %+v", debtTask)
	}
}

func TestLifeEventTriggerWorkflowEmitsEventToTaskGraphObservability(t *testing.T) {
	deps := buildPhase2Deps(t, safeHoldingsCSV, true, true)
	event := observation.LifeEventRecord{
		ID:         "event-job-1",
		UserID:     "user-1",
		Kind:       observation.LifeEventJobChange,
		Source:     "hris",
		Provenance: "job change fixture",
		ObservedAt: deps.Now,
		Confidence: 0.93,
		JobChange: &observation.JobChangeEventPayload{
			PreviousEmployer:             "OldCo",
			NewEmployer:                  "NextCo",
			PreviousMonthlyIncomeCents:   1000000,
			NewMonthlyIncomeCents:        1400000,
			BenefitsEnrollmentDeadlineAt: deps.Now.Add(7 * 24 * time.Hour),
		},
	}
	deadline := observation.CalendarDeadlineRecord{
		ID:               "deadline-job-1",
		UserID:           "user-1",
		Kind:             "benefits_enrollment",
		RelatedEventID:   event.ID,
		RelatedEventKind: event.Kind,
		Source:           "calendar",
		Provenance:       "job deadline fixture",
		ObservedAt:       deps.Now,
		DeadlineAt:       deps.Now.Add(7 * 24 * time.Hour),
		Description:      "Complete benefits enrollment after job change",
		Confidence:       0.92,
	}
	workflow := buildLifeEventWorkflow(t, deps, []observation.LifeEventRecord{event}, []observation.CalendarDeadlineRecord{deadline})

	result, err := workflow.Run(t.Context(), event, stateZero())
	if err != nil {
		t.Fatalf("run job change life event workflow: %v", err)
	}
	if result.RuntimeState != runtimepkg.WorkflowStateCompleted && result.RuntimeState != runtimepkg.WorkflowStateWaitingApproval {
		t.Fatalf("expected completed or approval-gated runtime state, got %q", result.RuntimeState)
	}

	entries := deps.EventLog.Entries()
	assertLogCategory(t, entries, "life_event_ingestion")
	assertLogCategory(t, entries, "life_event_state_diff")
	assertLogCategory(t, entries, "generated_task_graph")
	assertLogCategory(t, entries, "follow_up_tasks")
	assertLogCategory(t, entries, "life_event_follow_up_activation")
	assertLogCategory(t, entries, "life_event_follow_up_execution")
	taskEntry := findLogByCategory(entries, "follow_up_task_registered")
	if taskEntry == nil {
		t.Fatalf("expected per-task follow-up registration log entry")
	}
	if taskEntry.Details["required_capability"] == "" {
		t.Fatalf("expected follow-up registration log to include required_capability, got %+v", taskEntry)
	}
	if taskEntry.Details["status"] == string(runtimepkg.TaskQueueStatusQueuedPendingCapability) && taskEntry.Details["missing_capability_reason"] == "" {
		t.Fatalf("expected queued_pending_capability log to include missing_capability_reason, got %+v", taskEntry)
	}
	executionEntry := findLogByCategory(entries, "life_event_follow_up_execution")
	if executionEntry == nil || executionEntry.Details["executed_task_ids"] == "" {
		t.Fatalf("expected follow-up execution log entry with executed task ids, got %+v", executionEntry)
	}
}

func findFollowUpByIntent(t *testing.T, tasks []runtimepkg.FollowUpTaskRecord, intent taskspec.UserIntentType) runtimepkg.FollowUpTaskRecord {
	t.Helper()
	for _, item := range tasks {
		if item.Task.UserIntentType == intent {
			return item
		}
	}
	t.Fatalf("expected follow-up task with intent %q, got %+v", intent, tasks)
	return runtimepkg.FollowUpTaskRecord{}
}

func assertLogCategory(t *testing.T, entries []observability.LogEntry, category string) {
	t.Helper()
	if findLogByCategory(entries, category) == nil {
		t.Fatalf("expected log category %q in %+v", category, entries)
	}
}

func findLogByCategory(entries []observability.LogEntry, category string) *observability.LogEntry {
	for i := range entries {
		if entries[i].Category == category {
			return &entries[i]
		}
	}
	return nil
}
