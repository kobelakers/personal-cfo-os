import { useEffect, useState } from "react";
import { api } from "../lib/api";
import type { BenchmarkCompareView, BenchmarkRunDetail, BenchmarkRunSummary } from "../types";

export function useBenchmarks(selectedBenchmarkId: string) {
  const [benchmarkDetail, setBenchmarkDetail] = useState<BenchmarkRunDetail | null>(null);
  const [benchmarkCompare, setBenchmarkCompare] = useState<BenchmarkCompareView | null>(null);
  const [benchmarkMarkdown, setBenchmarkMarkdown] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!selectedBenchmarkId) {
      setBenchmarkDetail(null);
      setBenchmarkMarkdown("");
      return;
    }
    setLoading(true);
    setError("");
    void Promise.all([api.getBenchmark(selectedBenchmarkId), api.exportBenchmarkMarkdown(selectedBenchmarkId)])
      .then(([detail, markdown]) => {
        setBenchmarkDetail(detail);
        setBenchmarkMarkdown(markdown);
      })
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoading(false));
  }, [selectedBenchmarkId]);

  async function compareBenchmarks(benchmarks: BenchmarkRunSummary[]) {
    if (benchmarks.length < 2) {
      return;
    }
    const left = selectedBenchmarkId || benchmarks[0]?.id;
    const right = benchmarks.find((item) => item.id !== left)?.id || left;
    if (!left || !right) {
      return;
    }
    setLoading(true);
    setError("");
    try {
      setBenchmarkCompare(await api.compareBenchmarks(left, right));
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setLoading(false);
    }
  }

  return {
    benchmarkDetail,
    benchmarkCompare,
    benchmarkMarkdown,
    loading,
    error,
    compareBenchmarks,
    setBenchmarkCompare,
  };
}
