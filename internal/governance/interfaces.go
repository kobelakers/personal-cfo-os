package governance

import (
	"context"

	"github.com/kobelakers/personal-cfo-os/internal/memory"
	"github.com/kobelakers/personal-cfo-os/internal/state"
)

type PolicyEngine interface {
	EvaluateAction(request ActionRequest, approval ApprovalPolicy, toolPolicy *ToolExecutionPolicy) (PolicyDecision, AuditEvent, error)
	EvaluateMemoryWrite(record memory.MemoryRecord, policy MemoryWritePolicy, correlationID string) (PolicyDecision, AuditEvent, error)
	EvaluateReport(request ReportRequest, policy ReportDisclosurePolicy) (PolicyDecision, AuditEvent, error)
}

type RiskClassifier interface {
	Classify(current state.FinancialWorldState, action string) RiskAssessment
}

type MemoryWriteGate interface {
	AllowWrite(ctx context.Context, record memory.MemoryRecord) error
}
