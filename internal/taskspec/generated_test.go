package taskspec

import (
	"testing"
	"time"
)

func TestGeneratedTaskSpecWrapsExecutableTaskSpec(t *testing.T) {
	now := time.Date(2026, 3, 29, 9, 0, 0, 0, time.UTC)
	task := TaskSpec{
		ID:    "task-tax-1",
		Goal:  "Review tax optimization after salary change.",
		Scope: TaskScope{Areas: []string{"tax"}},
		Constraints: ConstraintSet{
			Hard: []string{"must remain evidence-backed"},
		},
		RiskLevel: RiskLevelMedium,
		SuccessCriteria: []SuccessCriteria{
			{ID: "coverage", Description: "task remains grounded in state diff and event evidence"},
		},
		RequiredEvidence: []RequiredEvidenceRef{
			{Type: "event_signal", Reason: "life event grounding", Mandatory: true},
		},
		ApprovalRequirement: ApprovalRequirementRecommended,
		UserIntentType:      UserIntentTaxOptimization,
		CreatedAt:           now,
	}
	generated := GeneratedTaskSpec{
		Task: task,
		Metadata: GeneratedTaskMetadata{
			GeneratedAt:      now,
			ParentWorkflowID: "workflow-life-event-1",
			ParentTaskID:     "task-life-event-1",
			TriggerSource:    TaskTriggerSourceLifeEvent,
			Priority:         TaskPriorityHigh,
		},
	}
	if err := generated.Validate(); err != nil {
		t.Fatalf("generated task should validate: %v", err)
	}
	if generated.ToTaskSpec().ID != task.ID {
		t.Fatalf("expected ToTaskSpec to preserve executable task contract")
	}
}

func TestTaskGraphValidateRejectsIncompatibleTaskContract(t *testing.T) {
	now := time.Date(2026, 3, 29, 9, 30, 0, 0, time.UTC)
	graph := TaskGraph{
		GraphID:          "graph-1",
		ParentWorkflowID: "workflow-life-event-1",
		ParentTaskID:     "task-life-event-1",
		TriggerSource:    TaskTriggerSourceLifeEvent,
		GeneratedAt:      now,
		GeneratedTasks: []GeneratedTaskSpec{
			{
				Task: TaskSpec{
					ID:                  "task-tax-1",
					Goal:                "Handle tax follow-up",
					Scope:               TaskScope{Areas: []string{"tax"}},
					Constraints:         ConstraintSet{Hard: []string{"deterministic only"}},
					RiskLevel:           RiskLevelMedium,
					SuccessCriteria:     []SuccessCriteria{{ID: "ok", Description: "ok"}},
					RequiredEvidence:    []RequiredEvidenceRef{{Type: "event_signal", Reason: "grounding", Mandatory: true}},
					ApprovalRequirement: ApprovalRequirementRecommended,
					UserIntentType:      UserIntentTaxOptimization,
					CreatedAt:           now,
				},
				Metadata: GeneratedTaskMetadata{
					GeneratedAt:      now,
					ParentWorkflowID: "workflow-life-event-1",
					ParentTaskID:     "task-life-event-1",
					TriggerSource:    TaskTriggerSourceLifeEvent,
					Priority:         TaskPriorityMedium,
				},
			},
		},
		Dependencies: []TaskDependency{
			{
				UpstreamTaskID:   "task-tax-1",
				DownstreamTaskID: "missing-task",
				Reason:           "portfolio task depends on tax review",
				Mandatory:        true,
			},
		},
	}
	if err := graph.Validate(); err == nil {
		t.Fatalf("expected missing downstream task to fail validation")
	}
}
