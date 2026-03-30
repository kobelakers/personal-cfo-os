# Benchmark Reporting

Phase 7B 把 benchmark/reporting 从“内部 eval 命令输出”整理成正式 operator/read surface。

## 当前来源

benchmark registry 现在统一读取两类来源，并做同一套 summary/detail 归一化：

- `docs/eval/samples` 里的 deterministic checked-in sample
- runtime / artifact plane 里的 `eval_run_result` artifact

P0 不支持 UI 触发 live eval run。  
这样可以保持 benchmark 面稳定、可复现、适合 README / 面试展示。

## Source / Dedupe

- `source=sample`：来自 checked-in deterministic sample
- `source=artifact`：来自 runtime/artifact plane
- registry 会优先保留 sample，再用 artifact 补齐 catalog
- 如果 sample 和 artifact 指向同一个 `run_id`，只显示一条，避免 operator surface 重复

## Cost-ish Summary

benchmark surface 现在会明确给出 `cost_summary`，并区分精度来源：

- `recorded_exact`
  - payload 显式自带 cost summary
- `estimated_from_usage`
  - payload 中已经包含 estimated cost 聚合
- `estimated_from_tokens`
  - eval run 本身没有 cost 字段时，按 token usage + deterministic token policy 估算

当前 token-policy fallback 不是账单系统，只是 operator/reporting 的 cost-ish 视图。  
它的目标是让 benchmark 面能同时回答：

- quality
- latency
- token usage
- cost-ish
- validator / governance outcome

## 输出面

### Run List

- corpus id
- source / artifact id
- pass/fail 汇总
- latency
- token usage
- cost-ish summary
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
- Markdown export 现在会带：
  - source
  - artifact id
  - cost-ish precision/source

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
7B closeout 则进一步把 benchmark 从“sample-dir 读取器”补成真正的 product/report surface，让 operator、README 和面试演示都能复用同一套证据包装。
