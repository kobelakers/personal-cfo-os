package runtime

import (
	"testing"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

func TestRegisterFollowUpTaskGraphPreservesTaskSpecContract(t *testing.T) {
	now := time.Date(2026, 3, 29, 10, 0, 0, 0, time.UTC)
	graph := taskspec.TaskGraph{
		GraphID:          "graph-1",
		ParentWorkflowID: "workflow-life-event-1",
		ParentTaskID:     "task-life-event-1",
		TriggerSource:    taskspec.TaskTriggerSourceLifeEvent,
		GeneratedAt:      now,
		GeneratedTasks: []taskspec.GeneratedTaskSpec{
			{
				Task: sampleGeneratedTaskSpec(now, taskspec.UserIntentMonthlyReview),
				Metadata: taskspec.GeneratedTaskMetadata{
					GeneratedAt:      now,
					ParentWorkflowID: "workflow-life-event-1",
					ParentTaskID:     "task-life-event-1",
					TriggerSource:    taskspec.TaskTriggerSourceLifeEvent,
					Priority:         taskspec.TaskPriorityHigh,
				},
			},
		},
	}
	result, err := RegisterFollowUpTaskGraph(graph, nil, now)
	if err != nil {
		t.Fatalf("register task graph: %v", err)
	}
	if len(result.RegisteredTasks) != 1 {
		t.Fatalf("expected one registered task, got %d", len(result.RegisteredTasks))
	}
	if result.RegisteredTasks[0].Task.UserIntentType != taskspec.UserIntentMonthlyReview {
		t.Fatalf("expected executable task spec to be preserved")
	}
	if result.RegisteredTasks[0].Status != TaskQueueStatusReady {
		t.Fatalf("expected monthly_review capability to be ready, got %q", result.RegisteredTasks[0].Status)
	}
}

func TestRegisterFollowUpTaskGraphMarksMissingCapabilityExplicitly(t *testing.T) {
	now := time.Date(2026, 3, 29, 10, 30, 0, 0, time.UTC)
	graph := taskspec.TaskGraph{
		GraphID:          "graph-2",
		ParentWorkflowID: "workflow-life-event-1",
		ParentTaskID:     "task-life-event-1",
		TriggerSource:    taskspec.TaskTriggerSourceLifeEvent,
		GeneratedAt:      now,
		GeneratedTasks: []taskspec.GeneratedTaskSpec{
			{
				Task: sampleGeneratedTaskSpec(now, taskspec.UserIntentPortfolioRebalance),
				Metadata: taskspec.GeneratedTaskMetadata{
					GeneratedAt:      now,
					ParentWorkflowID: "workflow-life-event-1",
					ParentTaskID:     "task-life-event-1",
					TriggerSource:    taskspec.TaskTriggerSourceLifeEvent,
					Priority:         taskspec.TaskPriorityHigh,
				},
			},
		},
	}
	result, err := RegisterFollowUpTaskGraph(graph, nil, now)
	if err != nil {
		t.Fatalf("register task graph: %v", err)
	}
	task := result.RegisteredTasks[0]
	if task.Status != TaskQueueStatusQueuedPendingCapability {
		t.Fatalf("expected queued_pending_capability, got %q", task.Status)
	}
	if task.RequiredCapability == "" || task.MissingCapabilityReason == "" {
		t.Fatalf("expected explicit capability metadata, got %+v", task)
	}
}

func sampleGeneratedTaskSpec(now time.Time, intent taskspec.UserIntentType) taskspec.TaskSpec {
	return taskspec.TaskSpec{
		ID:    "task-generated-" + string(intent),
		Goal:  "generated follow-up",
		Scope: taskspec.TaskScope{Areas: []string{"tax"}},
		Constraints: taskspec.ConstraintSet{
			Hard: []string{"must remain evidence-backed"},
		},
		RiskLevel: taskspec.RiskLevelMedium,
		SuccessCriteria: []taskspec.SuccessCriteria{
			{ID: "ok", Description: "ok"},
		},
		RequiredEvidence: []taskspec.RequiredEvidenceRef{
			{Type: "event_signal", Reason: "grounded follow-up task", Mandatory: true},
		},
		ApprovalRequirement: taskspec.ApprovalRequirementRecommended,
		UserIntentType:      intent,
		CreatedAt:           now,
	}
}
