import type { ReplayComparison, ReplayQueryScope, ReplayView } from "../types";
import { EmptyState, KeyValueGrid, Panel, SectionList } from "./common";

interface ReplayPanelProps {
  replayScope: ReplayQueryScope;
  replayID: string;
  compareLeft: string;
  compareRight: string;
  replayView: ReplayView | null;
  replayComparison: ReplayComparison | null;
  replayError: string;
  onChangeScope: (scope: ReplayQueryScope) => void;
  onChangeReplayID: (id: string) => void;
  onChangeCompareLeft: (value: string) => void;
  onChangeCompareRight: (value: string) => void;
  onQueryReplay: () => void;
  onCompareReplay: () => void;
}

export function ReplayPanel(props: ReplayPanelProps) {
  const {
    replayScope,
    replayID,
    compareLeft,
    compareRight,
    replayView,
    replayComparison,
    replayError,
    onChangeScope,
    onChangeReplayID,
    onChangeCompareLeft,
    onChangeCompareRight,
    onQueryReplay,
    onCompareReplay,
  } = props;

  return (
    <>
      <Panel title="Replay Query" subtitle="workflow / task-graph / task / execution / approval">
        <div className="form-grid">
          <label>
            <span>Scope</span>
            <select value={replayScope} onChange={(event) => onChangeScope(event.target.value as ReplayQueryScope)}>
              <option value="workflow">workflow</option>
              <option value="task_graph">task_graph</option>
              <option value="task">task</option>
              <option value="execution">execution</option>
              <option value="approval">approval</option>
            </select>
          </label>
          <label>
            <span>ID</span>
            <input value={replayID} onChange={(event) => onChangeReplayID(event.target.value)} placeholder="输入 replay scope id" />
          </label>
        </div>
        <div className="form-grid">
          <label>
            <span>Compare Left</span>
            <input value={compareLeft} onChange={(event) => onChangeCompareLeft(event.target.value)} placeholder="workflow:workflow-id" />
          </label>
          <label>
            <span>Compare Right</span>
            <input value={compareRight} onChange={(event) => onChangeCompareRight(event.target.value)} placeholder="approval:approval-id" />
          </label>
        </div>
        <div className="button-row">
          <button type="button" onClick={onQueryReplay}>
            Refresh Replay
          </button>
          <button type="button" onClick={onCompareReplay}>
            Compare
          </button>
        </div>
        {replayError ? <p className="inline-error">{replayError}</p> : null}
      </Panel>
      <Panel title="Replay Result" subtitle={replayView ? `${replayView.scope.kind}:${replayView.scope.id}` : "尚未查询"}>
        {replayView ? (
          <>
            <KeyValueGrid
              items={[
                ["Final State", replayView.summary.final_state || replayView.workflow?.runtime_state || "unknown"],
                ["Projection", replayView.projection_status || "n/a"],
                ["Projection Version", replayView.projection_version ? String(replayView.projection_version) : "n/a"],
                ["Degraded", String(replayView.degraded)],
              ]}
            />
            <SectionList title="Plan Summary" items={replayView.summary.plan_summary || []} />
            <SectionList title="Async Runtime Summary" items={replayView.summary.async_runtime_summary || []} />
            <SectionList title="Why / How" items={flattenReplayExplanation(replayView)} />
            <SectionList title="Provenance Summary" items={summarizeProvenance(replayView)} />
            <SectionList title="Degradation Reasons" items={replayView.degradation_reasons?.map((item) => `${item.reason} · ${item.message}`) || []} />
            {replayComparison ? (
              <SectionList title="Compare Summary" items={replayComparison.summary || replayComparison.diffs?.map((item) => `${item.category} · ${item.summary}`) || []} />
            ) : null}
          </>
        ) : (
          <EmptyState>输入 scope 与 id 后会显示 replay、why/how 与 async runtime summary。</EmptyState>
        )}
      </Panel>
    </>
  );
}

function flattenReplayExplanation(view: ReplayView): string[] {
  return [
    ...(view.explanation.why_skill_selected || []),
    ...(view.explanation.why_generated_task || []),
    ...(view.explanation.why_child_executed || []),
    ...(view.explanation.why_memory_decision || []),
    ...(view.explanation.why_validation_failed || []),
    ...(view.explanation.why_async_runtime || []),
    ...(view.explanation.why_failed ? [view.explanation.why_failed] : []),
    ...(view.explanation.why_waiting_approval ? [view.explanation.why_waiting_approval] : []),
  ];
}

function summarizeProvenance(view: ReplayView): string[] {
  const edges = view.provenance.edges || [];
  const nodes = view.provenance.nodes || [];
  return [`nodes=${nodes.length}`, `edges=${edges.length}`, ...edges.slice(0, 8).map((edge) => `${edge.type} · ${edge.from_node_id} -> ${edge.to_node_id}`)];
}
