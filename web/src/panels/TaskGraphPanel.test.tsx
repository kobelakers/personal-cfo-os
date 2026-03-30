import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { TaskGraphPanel } from "./TaskGraphPanel";

describe("TaskGraphPanel", () => {
  it("renders task graph list and detail blocks", () => {
    render(
      <TaskGraphPanel
        taskGraphs={[
          {
            snapshot: {
              graph: {
                graph_id: "graph-1",
                parent_workflow_id: "workflow-1",
                parent_task_id: "task-parent-1",
                generation_summary: "generated follow-ups",
                dependencies: [{ upstream_task_id: "task-a", downstream_task_id: "task-b", reason: "dependency" }],
              },
              registered_tasks: [{ task: { id: "task-a", goal: "goal", user_intent_type: "tax_optimization" }, status: "completed" }],
              deferred: [{ task_id: "task-b", reason: "window" }],
              executed_tasks: [{ execution_id: "exec-1", task_id: "task-a", workflow_id: "workflow-child-1", status: "completed" }],
            },
            artifacts: [],
            actions: [],
          },
        ]}
        selectedGraphId="graph-1"
        onSelectGraph={() => {}}
        taskGraphDetail={{
          snapshot: {
            graph: {
              graph_id: "graph-1",
              parent_workflow_id: "workflow-1",
              parent_task_id: "task-parent-1",
              generation_summary: "generated follow-ups",
              dependencies: [{ upstream_task_id: "task-a", downstream_task_id: "task-b", reason: "dependency" }],
            },
            registered_tasks: [{ task: { id: "task-a", goal: "goal", user_intent_type: "tax_optimization" }, status: "completed" }],
            deferred: [{ task_id: "task-b", reason: "window" }],
            executed_tasks: [{ execution_id: "exec-1", task_id: "task-a", workflow_id: "workflow-child-1", status: "completed" }],
          },
          artifacts: [],
          actions: [],
        }}
      />,
    );

    expect(screen.getByText("Task Graph Viewer")).toBeInTheDocument();
    expect(screen.getByText("generated follow-ups")).toBeInTheDocument();
    expect(screen.getByText("task-a · completed · tax_optimization")).toBeInTheDocument();
    expect(screen.getByText("task-a -> task-b · dependency")).toBeInTheDocument();
  });
});
