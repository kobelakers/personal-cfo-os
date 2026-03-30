# Operator UI

Phase 7B 把 `web/` 从 placeholder 升级成最小但正式的 operator console。  
这层只可视化已有 typed services、runtime、operator、replay、artifact、benchmark surfaces，不引入新的业务判断。

7B closeout 后，UI 结构也从单文件集中实现收口成 panelized structure：

- `TaskGraphPanel`
- `ApprovalPanel`
- `ReplayPanel`
- `IntelligencePanel`
- `ArtifactPanel`
- `BenchmarkPanel`

并配套最小 hooks：

- `useBootstrap`
- `useReplay`
- `useArtifact`
- `useBenchmarks`

这一步是工程化收口，不改变 UI 的核心行为，也不引入新的前端逻辑层。

## 设计边界

- UI 只调用 `/api/v1`
- UI 不直接连接 SQLite / Postgres / LocalFS / MinIO
- UI 不承担 planning、verification、governance 或 runtime orchestration
- 所有 approve / deny / resume / retry / reevaluate 仍然走 typed operator service

## 面板

### 1. Task Graph Viewer

- 查看 task graph 列表与详情
- 展示 task status、dependency、deferred、waiting_approval、queued_pending_capability
- 展示 child workflow linkage 和 graph artifacts

### 2. Approval Panel

- 展示 pending approvals
- 查看审批详情
- 提交 approve / deny
- 直接显示 conflict / stale version 错误，不在前端做重试或并发判断

### 3. Replay Viewer

- 支持 workflow / task-graph / task / execution / approval replay query
- 支持 compare
- 展示 async runtime summary、why/how explanation、provenance summary、degradation reasons

### 4. Memory / Skill / Runtime Summary

- 只显示已有 replay/debug surface：
- memory hits / writes / rejected memory
- selected skill family/version/recipe
- async runtime claim / reclaim / retry / approval-resume 摘要

### 5. Token / Cost / Report / Artifact

- 展示 prompt/provider/token/cost summary
- 展示 report artifact、replay bundle、checkpoint/report/replay payload refs
- artifact 内容通过 API 预览 typed JSON 或 summary

### 6. Benchmark Panel

- 展示 deterministic benchmark catalog
- 查看 detail、compare、export
- 不在 UI 中触发 live eval run

## 交付方式

- `interview-demo`：`cmd/api` 直接服务构建后的 `web/dist`
- `runtime-promotion`：`cmd/api` 直接服务构建后的 `web/dist`
- `dev-stack`：Vite 开发态，API 与 worker 独立运行

## 为什么这不是“把逻辑搬进前端”

- 所有 operator action 仍由后端 typed command/result 驱动
- 所有 replay/debug 仍来自 canonical `internal/runtime.ReplayQueryService`
- benchmark 仍来自 deterministic sample / eval artifact truth surface
- UI 只是可视化与动作触发层，不是新的协议真相源
- panelized structure 的目标是 maintainability，不是把判断逻辑抽到前端
