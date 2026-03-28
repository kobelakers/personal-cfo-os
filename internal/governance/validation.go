package governance

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/memory"
)

type StaticPolicyEngine struct{}

func (StaticPolicyEngine) EvaluateAction(request ActionRequest, approval ApprovalPolicy, toolPolicy *ToolExecutionPolicy) (PolicyDecision, AuditEvent, error) {
	if err := request.Validate(); err != nil {
		return PolicyDecision{}, AuditEvent{}, err
	}
	if err := approval.Validate(); err != nil {
		return PolicyDecision{}, AuditEvent{}, err
	}

	outcome := PolicyDecisionAllow
	reason := "action allowed by default"
	if toolPolicy != nil {
		if err := toolPolicy.Validate(); err != nil {
			return PolicyDecision{}, AuditEvent{}, err
		}
		if len(toolPolicy.AllowedRoles) > 0 && !hasIntersection(toolPolicy.AllowedRoles, request.ActorRoles) {
			outcome = PolicyDecisionDeny
			reason = "actor roles do not satisfy tool execution policy"
		}
		if compareRisk(request.RiskLevel, toolPolicy.RequiresApprovalAbove) >= 0 && outcome == PolicyDecisionAllow {
			outcome = PolicyDecisionRequireApproval
			reason = "tool execution policy requires approval"
		}
	}
	if compareRisk(request.RiskLevel, approval.MinRiskLevel) >= 0 && !approval.AutoApprove && outcome == PolicyDecisionAllow {
		outcome = PolicyDecisionRequireApproval
		reason = "approval policy requires human review"
	}
	if strings.Contains(request.Action, "single_stock") && compareRisk(request.RiskLevel, ActionRiskHigh) >= 0 && outcome == PolicyDecisionAllow {
		outcome = PolicyDecisionRequireApproval
		reason = "single-stock high-risk actions cannot auto-execute"
	}

	now := time.Now().UTC()
	decision := PolicyDecision{
		Outcome:       outcome,
		Reason:        reason,
		AppliedPolicy: approval.Name,
		EvaluatedAt:   now,
		AuditRef:      request.CorrelationID,
	}
	audit := AuditEvent{
		ID:            request.CorrelationID + "-action",
		Actor:         request.Actor,
		Action:        request.Action,
		Resource:      request.Resource,
		Outcome:       string(outcome),
		Reason:        reason,
		OccurredAt:    now,
		CorrelationID: request.CorrelationID,
	}
	return decision, audit, nil
}

func (StaticPolicyEngine) EvaluateMemoryWrite(record memory.MemoryRecord, policy MemoryWritePolicy, correlationID string) (PolicyDecision, AuditEvent, error) {
	if err := record.Validate(); err != nil {
		return PolicyDecision{}, AuditEvent{}, err
	}
	if err := policy.Validate(); err != nil {
		return PolicyDecision{}, AuditEvent{}, err
	}

	outcome := PolicyDecisionAllow
	reason := "memory write allowed"
	if record.Confidence.Score < policy.MinConfidence {
		outcome = PolicyDecisionDeny
		reason = "memory confidence below policy threshold"
	}
	if policy.RequireEvidence && len(record.Source.EvidenceIDs) == 0 {
		outcome = PolicyDecisionDeny
		reason = "memory write requires evidence provenance"
	}
	if len(policy.AllowKinds) > 0 && !containsMemoryKind(policy.AllowKinds, record.Kind) {
		outcome = PolicyDecisionDeny
		reason = "memory kind is not allowed by policy"
	}

	now := time.Now().UTC()
	decision := PolicyDecision{
		Outcome:       outcome,
		Reason:        reason,
		AppliedPolicy: "memory_write_policy",
		EvaluatedAt:   now,
		AuditRef:      correlationID,
	}
	audit := AuditEvent{
		ID:            correlationID + "-memory",
		Actor:         record.Source.Actor,
		Action:        "memory_write",
		Resource:      record.ID,
		Outcome:       string(outcome),
		Reason:        reason,
		OccurredAt:    now,
		CorrelationID: correlationID,
	}
	return decision, audit, nil
}

func (StaticPolicyEngine) EvaluateReport(request ReportRequest, policy ReportDisclosurePolicy) (PolicyDecision, AuditEvent, error) {
	if request.Actor == "" {
		return PolicyDecision{}, AuditEvent{}, errors.New("report actor is required")
	}
	if request.Audience == "" {
		return PolicyDecision{}, AuditEvent{}, errors.New("report audience is required")
	}
	outcome := PolicyDecisionAllow
	reason := "report can be disclosed"
	if request.ContainsPII && !policy.AllowPII {
		outcome = PolicyDecisionRedact
		reason = "report disclosure requires redaction"
	}
	now := time.Now().UTC()
	decision := PolicyDecision{
		Outcome:       outcome,
		Reason:        reason,
		AppliedPolicy: "report_disclosure_policy",
		EvaluatedAt:   now,
		AuditRef:      request.CorrelationID,
	}
	audit := AuditEvent{
		ID:            request.CorrelationID + "-report",
		Actor:         request.Actor,
		Action:        "report_disclosure",
		Resource:      request.Audience,
		Outcome:       string(outcome),
		Reason:        reason,
		OccurredAt:    now,
		CorrelationID: request.CorrelationID,
	}
	return decision, audit, nil
}

func (r ActionRequest) Validate() error {
	var errs []error
	if r.Actor == "" {
		errs = append(errs, errors.New("action actor is required"))
	}
	if r.Action == "" {
		errs = append(errs, errors.New("action name is required"))
	}
	if r.Resource == "" {
		errs = append(errs, errors.New("action resource is required"))
	}
	if !validActionRisk(r.RiskLevel) {
		errs = append(errs, fmt.Errorf("invalid action risk %q", r.RiskLevel))
	}
	if r.CorrelationID == "" {
		errs = append(errs, errors.New("correlation id is required"))
	}
	return errors.Join(errs...)
}

func (p ApprovalPolicy) Validate() error {
	if p.Name == "" {
		return errors.New("approval policy name is required")
	}
	if !validActionRisk(p.MinRiskLevel) {
		return fmt.Errorf("invalid approval policy min risk %q", p.MinRiskLevel)
	}
	return nil
}

func (p ToolExecutionPolicy) Validate() error {
	if p.ToolName == "" {
		return errors.New("tool execution policy tool name is required")
	}
	if !validActionRisk(p.RequiresApprovalAbove) {
		return fmt.Errorf("invalid tool execution approval risk %q", p.RequiresApprovalAbove)
	}
	if p.MaxCallsPerTask < 0 {
		return errors.New("max calls per task must be non-negative")
	}
	return nil
}

func (p MemoryWritePolicy) Validate() error {
	if p.MinConfidence < 0 || p.MinConfidence > 1 {
		return errors.New("memory write min confidence must be within [0,1]")
	}
	for _, kind := range p.AllowKinds {
		switch kind {
		case memory.MemoryKindEpisodic, memory.MemoryKindSemantic, memory.MemoryKindProcedural, memory.MemoryKindPolicy:
		default:
			return fmt.Errorf("invalid allowed memory kind %q", kind)
		}
	}
	return nil
}

func validActionRisk(level ActionRiskLevel) bool {
	switch level {
	case ActionRiskLow, ActionRiskMedium, ActionRiskHigh, ActionRiskCritical:
		return true
	default:
		return false
	}
}

func compareRisk(left, right ActionRiskLevel) int {
	rank := map[ActionRiskLevel]int{
		ActionRiskLow:      1,
		ActionRiskMedium:   2,
		ActionRiskHigh:     3,
		ActionRiskCritical: 4,
	}
	return rank[left] - rank[right]
}

func hasIntersection(a []string, b []string) bool {
	seen := make(map[string]struct{}, len(a))
	for _, item := range a {
		seen[item] = struct{}{}
	}
	for _, item := range b {
		if _, ok := seen[item]; ok {
			return true
		}
	}
	return false
}

func containsMemoryKind(items []memory.MemoryKind, target memory.MemoryKind) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
