# Protocol Exposure

Phase 7B 不是把 internal execution bus 对外公开，而是给系统一层 **versioned external legibility**。

## Canonical Surface

对外 canonical surface 是：

- `/api/v1`
- `schemas/public/v1/*`

现有 unversioned endpoints 继续保留，但只作为 compatibility alias。  
它们必须复用同一套 adapter/service 逻辑，不能形成第二份 handler 或第二份协议语义。

## External Exposure vs Internal Protocol

### External exposure

适合被文档化、被 operator UI 使用、被后续只读集成使用的对象：

- `TaskSpec`
- `ExecutionPlan`
- replay query / replay view
- operator action command / result
- report artifact
- benchmark run / compare

### Reference / internal execution protocol exposure

`AgentEnvelope` 会进入 versioned schema docs，但它的定位是：

- reference
- internal execution protocol exposure
- 帮助理解 system-agent / domain-agent 之间的 typed envelope

它**不是 public write contract**，也不是外部系统可直接写入 runtime 的入口。

## Versioning Strategy

- URL path versioning：`/api/v1`
- schema directory versioning：`schemas/public/v1`
- 新版本应通过新增目录和新 path 演进，而不是让外部依赖 internal package 细节
- compatibility alias 只保留现有行为，不再扩展叙事

## 本轮明确没做

- MCP server
- A2A server
- remote inbox/outbox
- public writable generic agent mailbox

这保持了 7B 的边界：让系统对外可解释、可集成，但不打穿当前 execution backbone。
