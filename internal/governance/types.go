package governance

import (
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/memory"
)

type ActionRiskLevel string

const (
	ActionRiskLow      ActionRiskLevel = "low"
	ActionRiskMedium   ActionRiskLevel = "medium"
	ActionRiskHigh     ActionRiskLevel = "high"
	ActionRiskCritical ActionRiskLevel = "critical"
)

type ApprovalPolicy struct {
	Name          string          `json:"name"`
	MinRiskLevel  ActionRiskLevel `json:"min_risk_level"`
	RequiredRoles []string        `json:"required_roles,omitempty"`
	AutoApprove   bool            `json:"auto_approve"`
}

type ToolExecutionPolicy struct {
	ToolName              string          `json:"tool_name"`
	AllowedRoles          []string        `json:"allowed_roles,omitempty"`
	EgressAllowlist       []string        `json:"egress_allowlist,omitempty"`
	MaxCallsPerTask       int             `json:"max_calls_per_task"`
	RequiresApprovalAbove ActionRiskLevel `json:"requires_approval_above"`
}

type MemoryWritePolicy struct {
	MinConfidence   float64             `json:"min_confidence"`
	RequireEvidence bool                `json:"require_evidence"`
	AllowKinds      []memory.MemoryKind `json:"allow_kinds,omitempty"`
}

type ReportDisclosurePolicy struct {
	Audience       string   `json:"audience"`
	AllowPII       bool     `json:"allow_pii"`
	RedactedFields []string `json:"redacted_fields,omitempty"`
}

type RoleBinding struct {
	Role     string   `json:"role"`
	Subjects []string `json:"subjects"`
}

type PolicyDecisionOutcome string

const (
	PolicyDecisionAllow           PolicyDecisionOutcome = "allow"
	PolicyDecisionDeny            PolicyDecisionOutcome = "deny"
	PolicyDecisionRequireApproval PolicyDecisionOutcome = "require_approval"
	PolicyDecisionRedact          PolicyDecisionOutcome = "redact"
	PolicyDecisionEscalate        PolicyDecisionOutcome = "escalate"
)

type PolicyDecision struct {
	Outcome       PolicyDecisionOutcome `json:"outcome"`
	Reason        string                `json:"reason"`
	AppliedPolicy string                `json:"applied_policy"`
	EvaluatedAt   time.Time             `json:"evaluated_at"`
	AuditRef      string                `json:"audit_ref"`
}

type AuditEvent struct {
	ID            string    `json:"id"`
	Actor         string    `json:"actor"`
	Action        string    `json:"action"`
	Resource      string    `json:"resource"`
	Outcome       string    `json:"outcome"`
	Reason        string    `json:"reason"`
	OccurredAt    time.Time `json:"occurred_at"`
	CorrelationID string    `json:"correlation_id"`
}

type ActionRequest struct {
	Actor         string          `json:"actor"`
	ActorRoles    []string        `json:"actor_roles,omitempty"`
	Action        string          `json:"action"`
	Resource      string          `json:"resource"`
	RiskLevel     ActionRiskLevel `json:"risk_level"`
	CorrelationID string          `json:"correlation_id"`
}

type ReportRequest struct {
	Actor         string `json:"actor"`
	Audience      string `json:"audience"`
	ContainsPII   bool   `json:"contains_pii"`
	CorrelationID string `json:"correlation_id"`
}
