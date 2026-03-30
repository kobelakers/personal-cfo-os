import { useEffect, useState } from "react";
import { api } from "../lib/api";
import type { ReplayComparison, ReplayQueryScope, ReplayView } from "../types";

interface UseReplayOptions {
  replayScope: ReplayQueryScope;
  replayID: string;
  artifactID: string;
  onArtifactDiscovered?: (artifactID: string) => void;
}

export function useReplay(options: UseReplayOptions) {
  const { replayScope, replayID, artifactID, onArtifactDiscovered } = options;
  const [replayView, setReplayView] = useState<ReplayView | null>(null);
  const [replayComparison, setReplayComparison] = useState<ReplayComparison | null>(null);
  const [replayError, setReplayError] = useState("");
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!replayID) {
      setReplayView(null);
      return;
    }
    void queryReplay();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [replayScope, replayID]);

  async function queryReplay() {
    if (!replayID) {
      setReplayError("请输入 replay id");
      return;
    }
    setLoading(true);
    setReplayError("");
    try {
      const view = await api.getReplay(replayScope, replayID);
      setReplayView(view);
      const nextArtifact = firstArtifactID(view);
      if (nextArtifact && !artifactID) {
        onArtifactDiscovered?.(nextArtifact);
      }
    } catch (err) {
      setReplayError((err as Error).message);
    } finally {
      setLoading(false);
    }
  }

  async function compareReplay(left: string, right: string) {
    if (!left || !right) {
      setReplayError("compare 需要 left 和 right 两个 scope:id");
      return;
    }
    setLoading(true);
    setReplayError("");
    try {
      setReplayComparison(await api.compareReplay(left, right));
    } catch (err) {
      setReplayError((err as Error).message);
    } finally {
      setLoading(false);
    }
  }

  return {
    replayView,
    replayComparison,
    replayError,
    loading,
    queryReplay,
    compareReplay,
    setReplayView,
    setReplayComparison,
    setReplayError,
  };
}

function firstArtifactID(view: ReplayView): string {
  const items = view.workflow?.artifacts || view.task_graph?.artifacts || [];
  return items[0]?.id || "";
}
