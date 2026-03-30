# ADR 0016: Productization and Versioned Operator Exposure

## Status

Accepted

## Context

在 7A closeout 之后，Personal CFO OS 已经具备：

- closeout-hardened async runtime backbone
- canonical replay/eval/debug plane
- formal skill runtime
- formal behavior domain

但系统仍主要通过 CLI/JSON 被使用。  
这会带来两个问题：

1. operator usability 偏弱，面试或演示时需要大量命令行上下文
2. external legibility 不足，外部只读集成边界不清楚，容易把 internal execution protocol 误解成 public contract

## Decision

Phase 7B 采取以下决策：

1. 新增最小但正式的 operator UI，作为现有 typed services 的可视化层
2. 引入 `/api/v1` 作为 canonical operator/read surface
3. 保留现有 unversioned endpoints，但只作为 compatibility alias，必须复用同一套 adapter/service 逻辑
4. 在 `schemas/public/v1` 下提供 versioned schema docs
5. 将 `AgentEnvelope` 仅文档化为 reference/internal execution protocol exposure，而不是 public write contract
6. 把 benchmark/reporting 整理成正式读/比较/导出面，不在 UI 中触发 live eval run
7. 把运行方式整理成 `local-lite / runtime-promotion / interview-demo / dev-stack` 四个 profile

## Consequences

### Positive

- operator 不再只能通过 CLI 使用系统
- `/api/v1` 提供清晰、可文档化的 canonical surface
- deployment profile 更适合面试、演示、开发、runtime proof 不同场景
- benchmark/reporting 成为正式产品输出，而不是内部命令副产物

### Negative

- 需要额外维护 UI、product docs 和 public schema docs
- compatibility alias 仍需保留一段时间，增加少量路由维护成本

## Guardrails

- UI 不承载业务逻辑
- API adapter 不复制内核判断
- replay truth 继续唯一指向 canonical `internal/runtime.ReplayQueryService`
- external exposure 不等于 external writable protocol
- 本 ADR 不引入 MCP server、A2A server、remote agent mailbox、broker 或 live Temporal cluster
