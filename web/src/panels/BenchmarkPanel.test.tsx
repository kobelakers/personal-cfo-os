import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { BenchmarkPanel } from "./BenchmarkPanel";

describe("BenchmarkPanel", () => {
  it("renders source and cost-ish summary", () => {
    render(
      <BenchmarkPanel
        benchmarks={[
          {
            id: "phase6b",
            source: "sample",
            title: "phase6b",
            corpus_id: "phase6b-default",
            run_id: "phase6b-run",
            deterministic_only: true,
            scenario_count: 4,
            passed_count: 4,
            failed_count: 0,
            approval_frequency: 0.25,
            average_latency_ms: 42,
            total_token_usage: 780,
            validator_pass_rate: 1,
            policy_violation_rate: 0,
            cost_summary: { usd: 0.0142, precision: "estimated_from_tokens", source: "token_policy_v1" },
          },
        ]}
        selectedBenchmarkId="phase6b"
        onSelectBenchmark={() => {}}
        benchmarkDetail={{
          summary: {
            id: "phase6b",
            source: "sample",
            title: "phase6b",
            corpus_id: "phase6b-default",
            run_id: "phase6b-run",
            deterministic_only: true,
            scenario_count: 4,
            passed_count: 4,
            failed_count: 0,
            approval_frequency: 0.25,
            average_latency_ms: 42,
            total_token_usage: 780,
            validator_pass_rate: 1,
            policy_violation_rate: 0,
            cost_summary: { usd: 0.0142, precision: "estimated_from_tokens", source: "token_policy_v1" },
          },
          run: {
            summary: { summary_lines: ["phase6b deterministic corpus"] },
            results: [{ scenario_id: "behavior_intervention_happy_path", passed: true, runtime_state: "completed", token_usage: 240, duration_milliseconds: 40 }],
          },
        }}
        benchmarkCompare={null}
        benchmarkMarkdown=""
        onCompareBenchmarks={() => {}}
      />,
    );

    expect(screen.getByText("sample")).toBeInTheDocument();
    expect(screen.getByText("Cost Summary")).toBeInTheDocument();
    expect(screen.getByText("precision=estimated_from_tokens")).toBeInTheDocument();
  });
});
