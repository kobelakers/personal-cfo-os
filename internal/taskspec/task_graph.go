package taskspec

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type TaskDependency struct {
	UpstreamTaskID   string `json:"upstream_task_id"`
	DownstreamTaskID string `json:"downstream_task_id"`
	Reason           string `json:"reason"`
	Mandatory        bool   `json:"mandatory"`
}

type TaskGraph struct {
	GraphID           string              `json:"graph_id"`
	ParentWorkflowID  string              `json:"parent_workflow_id"`
	ParentTaskID      string              `json:"parent_task_id"`
	TriggerSource     TaskTriggerSource   `json:"trigger_source"`
	GeneratedAt       time.Time           `json:"generated_at"`
	GeneratedTasks    []GeneratedTaskSpec `json:"generated_tasks"`
	Dependencies      []TaskDependency    `json:"dependencies,omitempty"`
	SuppressionNotes  []string            `json:"suppression_notes,omitempty"`
	GenerationSummary string              `json:"generation_summary,omitempty"`
}

func (g TaskGraph) Validate() error {
	var errs []error
	if strings.TrimSpace(g.GraphID) == "" {
		errs = append(errs, errors.New("task graph graph_id is required"))
	}
	if strings.TrimSpace(g.ParentWorkflowID) == "" {
		errs = append(errs, errors.New("task graph parent_workflow_id is required"))
	}
	if strings.TrimSpace(g.ParentTaskID) == "" {
		errs = append(errs, errors.New("task graph parent_task_id is required"))
	}
	if !validTaskTriggerSource(g.TriggerSource) {
		errs = append(errs, errors.New("task graph trigger_source is required"))
	}
	if g.GeneratedAt.IsZero() {
		errs = append(errs, errors.New("task graph generated_at is required"))
	}
	if len(g.GeneratedTasks) == 0 {
		errs = append(errs, errors.New("task graph requires at least one generated task"))
	}

	taskIDs := make(map[string]struct{}, len(g.GeneratedTasks))
	for i, task := range g.GeneratedTasks {
		if err := task.Validate(); err != nil {
			errs = append(errs, fmt.Errorf("generated task %d invalid: %w", i, err))
			continue
		}
		if task.Metadata.ParentWorkflowID != g.ParentWorkflowID {
			errs = append(errs, fmt.Errorf("generated task %d parent_workflow_id mismatch", i))
		}
		if task.Metadata.ParentTaskID != g.ParentTaskID {
			errs = append(errs, fmt.Errorf("generated task %d parent_task_id mismatch", i))
		}
		if _, exists := taskIDs[task.Task.ID]; exists {
			errs = append(errs, fmt.Errorf("duplicate generated task id %q in task graph", task.Task.ID))
			continue
		}
		taskIDs[task.Task.ID] = struct{}{}
	}

	for i, dep := range g.Dependencies {
		if strings.TrimSpace(dep.UpstreamTaskID) == "" || strings.TrimSpace(dep.DownstreamTaskID) == "" {
			errs = append(errs, fmt.Errorf("task dependency %d requires upstream and downstream ids", i))
			continue
		}
		if dep.UpstreamTaskID == dep.DownstreamTaskID {
			errs = append(errs, fmt.Errorf("task dependency %d cannot self-reference %q", i, dep.UpstreamTaskID))
		}
		if _, ok := taskIDs[dep.UpstreamTaskID]; !ok {
			errs = append(errs, fmt.Errorf("task dependency %d references unknown upstream task %q", i, dep.UpstreamTaskID))
		}
		if _, ok := taskIDs[dep.DownstreamTaskID]; !ok {
			errs = append(errs, fmt.Errorf("task dependency %d references unknown downstream task %q", i, dep.DownstreamTaskID))
		}
		if strings.TrimSpace(dep.Reason) == "" {
			errs = append(errs, fmt.Errorf("task dependency %d requires a reason", i))
		}
	}

	return errors.Join(errs...)
}
