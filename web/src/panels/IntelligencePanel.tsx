import type { ReplayView } from "../types";
import { Panel, SectionList } from "./common";

interface IntelligencePanelProps {
  replayView: ReplayView | null;
}

export function IntelligencePanel(props: IntelligencePanelProps) {
  const { replayView } = props;
  return (
    <>
      <Panel title="Memory Summary" subtitle="selected / rejected / written">
        <SectionList title="Memory" items={replayView?.summary.memory_summary || []} />
        <SectionList title="Why Memory" items={replayView?.explanation.why_memory_decision || []} />
      </Panel>
      <Panel title="Skill + Runtime Summary" subtitle="skill selection、claim/reclaim/retry/resume">
        <SectionList title="Skill" items={replayView?.summary.skill_summary || []} />
        <SectionList title="Why Skill" items={replayView?.explanation.why_skill_selected || []} />
        <SectionList title="Runtime" items={replayView?.summary.async_runtime_summary || []} />
        <SectionList title="Why Runtime" items={replayView?.explanation.why_async_runtime || []} />
      </Panel>
    </>
  );
}
