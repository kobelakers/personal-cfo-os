import type { ArtifactContentView, ArtifactMeta, ReplayArtifactRef } from "../types";
import { EmptyState, KeyValueGrid, Panel, SectionList } from "./common";

interface ArtifactPanelProps {
  artifactCandidates: Array<ArtifactMeta | ReplayArtifactRef>;
  artifactID: string;
  onSelectArtifact: (artifactID: string) => void;
  artifactView: ArtifactContentView | null;
  artifactError: string;
}

export function ArtifactPanel(props: ArtifactPanelProps) {
  const { artifactCandidates, artifactID, onSelectArtifact, artifactView, artifactError } = props;
  return (
    <>
      <Panel title="Artifacts" subtitle="report / replay bundle / checkpoint refs">
        <ul className="list">
          {artifactCandidates.map((item) => (
            <li key={item.id}>
              <button
                className={item.id === artifactID ? "list-button is-active" : "list-button"}
                type="button"
                onClick={() => onSelectArtifact(item.id)}
              >
                <strong>{item.kind}</strong>
                <span>{artifactCandidateLabel(item)}</span>
              </button>
            </li>
          ))}
        </ul>
        {artifactError ? <p className="inline-error">{artifactError}</p> : null}
      </Panel>
      <Panel title="Token / Cost / Report / Artifact Detail" subtitle={artifactView?.artifact.id || "选择一个 artifact"}>
        {artifactView ? (
          <>
            <KeyValueGrid
              items={[
                ["Kind", artifactView.artifact.kind],
                ["Workflow", artifactView.artifact.workflow_id],
                ["Task", artifactView.artifact.task_id],
                ["Location", artifactView.artifact.ref?.location || "n/a"],
                ["Total Tokens", String(artifactView.usage_summary?.total_tokens || 0)],
                ["Estimated Cost", artifactView.usage_summary?.estimated_cost_usd?.toFixed(4) || "0.0000"],
              ]}
            />
            <SectionList title="Prompt / Provider / Cost Evidence" items={artifactPromptSummary(artifactView)} />
            <SectionList title="Artifact Summary" items={artifactView.summary_lines || []} />
            <pre className="code-block">{JSON.stringify(artifactView.structured ?? artifactView.raw_text ?? {}, null, 2)}</pre>
          </>
        ) : (
          <EmptyState>从 replay 或 task graph 里选择一个 artifact 查看 payload 和 token/cost 摘要。</EmptyState>
        )}
      </Panel>
    </>
  );
}

function artifactPromptSummary(view: ArtifactContentView): string[] {
  const items: string[] = [];
  if (view.usage_summary?.providers?.length) {
    items.push(`providers=${view.usage_summary.providers.join(",")}`);
  }
  if (view.usage_summary?.models?.length) {
    items.push(`models=${view.usage_summary.models.join(",")}`);
  }
  if (view.usage_summary?.prompt_ids?.length) {
    items.push(`prompt_ids=${view.usage_summary.prompt_ids.join(",")}`);
  }
  if (view.usage_summary?.call_count) {
    items.push(`call_count=${view.usage_summary.call_count}`);
  }
  return items;
}

function artifactCandidateLabel(item: { id: string; ref?: { location?: string; summary?: string }; summary?: string; location?: string }) {
  return item.summary || item.location || item.ref?.summary || item.ref?.location || item.id;
}
