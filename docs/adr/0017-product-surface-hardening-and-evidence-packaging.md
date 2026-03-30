# ADR 0017: Product Surface Hardening and Evidence Packaging

## Status

Accepted

## Context

Phase 7B 主体已经完成：

- operator UI 已成立
- `/api/v1` 已成为 canonical operator/read surface
- deployment profiles 已整理
- `schemas/public/v1/*` 已成立
- benchmark/reporting 已有 list/detail/compare/export

但 closeout 前仍有几个面试和工程可辩护性缺口：

1. benchmark registry 过于依赖 checked-in sample 目录
2. cost-ish summary 还不够闭合，容易被追问“为什么总是 0 或没有来源说明”
3. `web/src/App.tsx` 过于集中，不像工程化产品层
4. 7B 的 demo / smoke / sample 入口还不够像 operator-facing evidence pack

## Decision

7B closeout 采取以下收口决策：

1. benchmark registry 统一支持两类来源：
   - checked-in deterministic samples
   - runtime/artifact plane 中的 `eval_run_result`
2. benchmark run summary/detail 统一增加 source / artifact_id / cost-ish summary
3. cost-ish summary 的优先级为：
   - 显式 recorded cost summary
   - payload 中已有的 estimated cost
   - token usage + deterministic token policy estimate
4. operator UI 保持行为不变，但从单文件大组件收口成 panelized structure + minimal hooks
5. 补齐 7B runbook / sample index / evidence docs，让 operator/demo/interview 路径更清晰

## Consequences

### Positive

- benchmark surface 更接近 operator-grade，而不是 sample-dir 读取器
- cost-ish 不再是占位信息，source 和精度边界更清楚
- UI 代码更像产品层而不是临时交付层
- 7B 的演示和证据包装更适合面试、README、仓库交接

### Negative

- product 层代码和文档会多一层维护成本
- benchmark registry 仍然需要处理 sample 与 artifact 的 dedupe 策略

## Guardrails

- benchmark registry 不引入第二套 truth source
- replay truth 仍然唯一指向 canonical replay plane
- UI 仍然只是 view / action trigger 层
- 本 ADR 不开启新 phase，不扩 workflow，不进入 live eval control plane
