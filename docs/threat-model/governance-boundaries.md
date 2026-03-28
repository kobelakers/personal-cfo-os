# Governance Boundaries

## Primary Risks

- High-risk recommendations without approval
- Sensitive memory writes with weak provenance
- Tool execution outside allowed roles or egress boundaries
- Reports exposing personal data without disclosure review
- Runtime retries masking policy failures or evidence gaps

## Phase 1 Controls

- `ApprovalPolicy`
- `ToolExecutionPolicy`
- `MemoryWritePolicy`
- `ReportDisclosurePolicy`
- `PolicyDecision`
- `AuditEvent`

These controls define the enforcement surface even before the actual tool adapters and UI approval panel are fully implemented.
