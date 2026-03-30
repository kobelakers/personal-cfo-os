import type { TaskGraphView } from "../types";
import { EmptyState, KeyValueGrid, Panel, SectionList } from "./common";

interface TaskGraphPanelProps {
  taskGraphs: TaskGraphView[];
  selectedGraphId: string;
  onSelectGraph: (graphId: string) => void;
  taskGraphDetail: TaskGraphView | null;
}

export function TaskGraphPanel(props: TaskGraphPanelProps) {
  const { taskGraphs, selectedGraphId, onSelectGraph, taskGraphDetail } = props;
  return (
    <>
      <Panel title="Task Graph Viewer" subtitle="task graph、dependency、deferred、child workflow">
        <ul className="list">
          {taskGraphs.map((item) => {
            const graphId = item.snapshot.graph.graph_id;
            return (
              <li key={graphId}>
                <button
                  className={graphId === selectedGraphId ? "list-button is-active" : "list-button"}
                  type="button"
                  onClick={() => onSelectGraph(graphId)}
                >
                  <strong>{graphId}</strong>
                  <span>{item.snapshot.graph.generation_summary || item.snapshot.graph.parent_workflow_id}</span>
                </button>
              </li>
            );
          })}
        </ul>
      </Panel>
      <Panel title="Task Graph Detail" subtitle={taskGraphDetail?.snapshot.graph.graph_id || "选择一个 task graph"}>
        {taskGraphDetail ? (
          <>
            <KeyValueGrid
              items={[
                ["Parent Workflow", taskGraphDetail.snapshot.graph.parent_workflow_id],
                ["Parent Task", taskGraphDetail.snapshot.graph.parent_task_id],
                ["Pending Approval", taskGraphDetail.pending_approval?.approval_id || "none"],
                ["Executed Tasks", String(taskGraphDetail.snapshot.executed_tasks?.length || 0)],
              ]}
            />
            <SectionList
              title="Registered Tasks"
              items={taskGraphDetail.snapshot.registered_tasks?.map((item) => `${item.task.id} · ${item.status} · ${item.task.user_intent_type}`) || []}
            />
            <SectionList
              title="Dependencies"
              items={taskGraphDetail.snapshot.graph.dependencies?.map((item) => `${item.upstream_task_id} -> ${item.downstream_task_id} · ${item.reason}`) || []}
            />
            <SectionList title="Deferred" items={taskGraphDetail.snapshot.deferred?.map((item) => `${item.task_id} · ${item.reason}`) || []} />
          </>
        ) : (
          <EmptyState>选择左侧 task graph 查看详情。</EmptyState>
        )}
      </Panel>
    </>
  );
}
