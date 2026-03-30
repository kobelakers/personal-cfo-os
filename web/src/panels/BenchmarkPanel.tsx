import type { BenchmarkCompareView, BenchmarkRunDetail, BenchmarkRunSummary } from "../types";
import { EmptyState, KeyValueGrid, Panel, SectionList } from "./common";

interface BenchmarkPanelProps {
  benchmarks: BenchmarkRunSummary[];
  selectedBenchmarkId: string;
  onSelectBenchmark: (benchmarkID: string) => void;
  benchmarkDetail: BenchmarkRunDetail | null;
  benchmarkCompare: BenchmarkCompareView | null;
  benchmarkMarkdown: string;
  onCompareBenchmarks: () => void;
}

export function BenchmarkPanel(props: BenchmarkPanelProps) {
  const { benchmarks, selectedBenchmarkId, onSelectBenchmark, benchmarkDetail, benchmarkCompare, benchmarkMarkdown, onCompareBenchmarks } = props;
  return (
    <>
      <Panel title="Benchmark Catalog" subtitle="deterministic corpus / export-ready">
        <ul className="list">
          {benchmarks.map((item) => (
            <li key={item.id}>
              <button
                className={item.id === selectedBenchmarkId ? "list-button is-active" : "list-button"}
                type="button"
                onClick={() => onSelectBenchmark(item.id)}
              >
                <strong>{item.id}</strong>
                <span>
                  {item.corpus_id} · {item.source} · passed {item.passed_count}/{item.scenario_count}
                </span>
              </button>
            </li>
          ))}
        </ul>
        <div className="button-row">
          <button type="button" onClick={onCompareBenchmarks}>
            Compare First Two
          </button>
        </div>
      </Panel>
      <Panel title="Benchmark Detail" subtitle={benchmarkDetail?.summary.id || "选择一个 benchmark"}>
        {benchmarkDetail ? (
          <>
            <KeyValueGrid
              items={[
                ["Corpus", benchmarkDetail.summary.corpus_id],
                ["Source", benchmarkDetail.summary.source],
                ["Artifact", benchmarkDetail.summary.artifact_id || "n/a"],
                ["Deterministic", String(benchmarkDetail.summary.deterministic_only)],
                ["Passed", `${benchmarkDetail.summary.passed_count}/${benchmarkDetail.summary.scenario_count}`],
                ["Latency", `${benchmarkDetail.summary.average_latency_ms.toFixed(2)} ms`],
                ["Tokens", String(benchmarkDetail.summary.total_token_usage)],
                ["Cost-ish", `${benchmarkDetail.summary.cost_summary.usd.toFixed(6)} · ${benchmarkDetail.summary.cost_summary.precision}`],
              ]}
            />
            <SectionList title="Summary Lines" items={benchmarkDetail.run.summary.summary_lines || []} />
            <SectionList title="Cost Summary" items={benchmarkCostSummaryLines(benchmarkDetail.summary)} />
            <SectionList title="Scenario Status" items={benchmarkDetail.run.results.map((item) => `${item.scenario_id} · ${item.passed ? "passed" : "failed"} · ${item.runtime_state}`)} />
            {benchmarkCompare ? <SectionList title="Compare Summary" items={benchmarkCompare.summary || benchmarkCompare.diff.summary || []} /> : null}
            {benchmarkMarkdown ? <pre className="code-block">{benchmarkMarkdown}</pre> : null}
          </>
        ) : (
          <EmptyState>benchmarks panel 只做 deterministic run 的读、比较和导出，不在 UI 中触发 live eval。</EmptyState>
        )}
      </Panel>
    </>
  );
}

function benchmarkCostSummaryLines(summary: BenchmarkRunSummary): string[] {
  return [
    `usd=${summary.cost_summary.usd.toFixed(6)}`,
    `precision=${summary.cost_summary.precision}`,
    `source=${summary.cost_summary.source || "unknown"}`,
  ];
}
