import { useCallback, useEffect, useState } from "react";
import { api } from "../lib/api";
import type { ApprovalRecord, BenchmarkRunSummary, ProfileMeta, TaskGraphView } from "../types";

interface UseBootstrapOptions {
  onLoaded?: (payload: { profile: ProfileMeta; taskGraphs: TaskGraphView[]; approvals: ApprovalRecord[]; benchmarks: BenchmarkRunSummary[] }) => void;
}

export function useBootstrap(options: UseBootstrapOptions = {}) {
  const [profile, setProfile] = useState<ProfileMeta | null>(null);
  const [taskGraphs, setTaskGraphs] = useState<TaskGraphView[]>([]);
  const [approvals, setApprovals] = useState<ApprovalRecord[]>([]);
  const [benchmarks, setBenchmarks] = useState<BenchmarkRunSummary[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const [meta, graphList, approvalList, benchmarkList] = await Promise.all([
        api.profile(),
        api.listTaskGraphs(),
        api.listPendingApprovals(),
        api.listBenchmarks(),
      ]);
      setProfile(meta);
      setTaskGraphs(graphList);
      setApprovals(approvalList);
      setBenchmarks(benchmarkList);
      options.onLoaded?.({ profile: meta, taskGraphs: graphList, approvals: approvalList, benchmarks: benchmarkList });
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setLoading(false);
    }
  }, [options]);

  useEffect(() => {
    void load();
  }, [load]);

  return {
    profile,
    taskGraphs,
    approvals,
    benchmarks,
    loading,
    error,
    reload: load,
    setTaskGraphs,
    setApprovals,
    setBenchmarks,
  };
}
