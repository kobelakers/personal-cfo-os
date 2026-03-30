# Benchmark Reporting

Phase 7B 把 benchmark/reporting 从“内部 eval 命令输出”整理成正式 operator/read surface。

## 当前来源

benchmark registry 目前优先读取：

- `docs/eval/samples` 里的 deterministic checked-in sample
- artifact kind = `eval_run_result` 的正式产物（后续可继续扩）

P0 不支持 UI 触发 live eval run。  
这样可以保持 benchmark 面稳定、可复现、适合 README / 面试展示。

## 输出面

### Run List

- corpus id
- pass/fail 汇总
- latency
- token usage
- approval frequency
- validator / governance 汇总指标

### Run Detail

- scenario 级结果
- summary lines
- runtime state
- regression failure 摘要

### Compare

- 比较两次 deterministic run
- 输出 readable summary
- 展示 replay diff / validator / governance 差异摘要

### Export

- JSON：machine-readable
- Markdown/summary：human-readable、适合 docs / README / 面试展示

## API

- `GET /api/v1/benchmarks/runs`
- `GET /api/v1/benchmarks/runs/{id}`
- `GET /api/v1/benchmarks/compare?left=...&right=...`
- `GET /api/v1/benchmarks/exports/{id}?format=json|md`

## UI 边界

- benchmark panel 只做读、比较、导出
- 不提供 live eval run 按钮
- 不把 eval harness 逻辑搬进前端

## 为什么这一步重要

7A 证明的是 runtime backbone；7B 要把这些 proof 和 deterministic corpus 变成可展示、可解释、可复用的正式输出面。  
这样项目就不只是“能在命令行里跑”，而是有了 operator-usable 和 externally legible 的 benchmark/reporting surface。
