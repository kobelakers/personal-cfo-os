package governance

import "github.com/kobelakers/personal-cfo-os/internal/memory"

type PolicyEngine interface {
	EvaluateAction(request ActionRequest, approval ApprovalPolicy, toolPolicy *ToolExecutionPolicy) (PolicyDecision, AuditEvent, error)
	EvaluateMemoryWrite(record memory.MemoryRecord, policy MemoryWritePolicy, correlationID string) (PolicyDecision, AuditEvent, error)
	EvaluateReport(request ReportRequest, policy ReportDisclosurePolicy) (PolicyDecision, AuditEvent, error)
}
