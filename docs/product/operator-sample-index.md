# Operator Sample Index

7B closeout 之后，推荐把下面这组文件当成 operator-facing evidence pack 使用。

## Benchmark / Reporting

- [`docs/eval/samples/phase7b_benchmark_summary.json`](../eval/samples/phase7b_benchmark_summary.json)
  - benchmark list surface
  - 适合展示 source / cost-ish / pass/fail / latency / approval frequency

- [`docs/eval/samples/phase7b_benchmark_compare.json`](../eval/samples/phase7b_benchmark_compare.json)
  - benchmark compare surface
  - 适合展示 corpus/profile/config 比较摘要

## Operator Surface

- [`docs/eval/samples/phase7b_operator_surface.json`](../eval/samples/phase7b_operator_surface.json)
  - operator-facing surface index
  - 适合说明 `/api/v1`、panel 划分、canonical surface 边界

## Interview Demo Seed

- `var/interview-demo/demo-manifest.json`
  - 由 `cmd/demo-seed` 生成
  - 适合说明 interview-demo 实际 seed 了哪些 workflow / approval / task graph

## Protocol / External Legibility

- [`schemas/public/v1/taskspec.schema.json`](../../schemas/public/v1/taskspec.schema.json)
- [`schemas/public/v1/replay-view.schema.json`](../../schemas/public/v1/replay-view.schema.json)
- [`schemas/public/v1/operator-action-command.schema.json`](../../schemas/public/v1/operator-action-command.schema.json)
- [`schemas/public/v1/benchmark-run.schema.json`](../../schemas/public/v1/benchmark-run.schema.json)
- [`schemas/public/v1/agent-envelope.schema.json`](../../schemas/public/v1/agent-envelope.schema.json)
  - 注意：`agent-envelope` 只作 reference/internal execution protocol exposure

## 建议演示面板

1. `Task Graph Viewer`
2. `Approval Panel`
3. `Replay Viewer`
4. `Memory / Skill / Runtime`
5. `Artifacts`
6. `Benchmarks`
