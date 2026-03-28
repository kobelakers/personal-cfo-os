package governance

import (
	"context"
	"fmt"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/memory"
	"github.com/kobelakers/personal-cfo-os/internal/state"
)

type ApprovalEvaluation struct {
	RiskAssessment RiskAssessment  `json:"risk_assessment"`
	Decision       *PolicyDecision `json:"decision,omitempty"`
	Audit          *AuditEvent     `json:"audit,omitempty"`
}

type ReportEvaluation struct {
	Decision PolicyDecision `json:"decision"`
	Audit    AuditEvent     `json:"audit"`
}

type ApprovalService struct {
	Classifier     RiskClassifier
	Decider        ApprovalDecider
	PolicyEngine   PolicyEngine
	ApprovalPolicy ApprovalPolicy
	ToolPolicy     *ToolExecutionPolicy
	ReportPolicy   ReportDisclosurePolicy
}

func (s ApprovalService) EvaluateAction(
	current state.FinancialWorldState,
	workflowID string,
	action string,
	resource string,
	actor string,
	actorRoles []string,
	forceApproval bool,
) (ApprovalEvaluation, error) {
	classifier := s.classifier()
	risk := classifier.Classify(current, action)
	approvalPolicy := s.approvalPolicy()
	decision, audit, err := s.decider().Decide(ActionRequest{
		Actor:         actor,
		ActorRoles:    actorRoles,
		Action:        action,
		Resource:      resource,
		RiskLevel:     risk.Level,
		CorrelationID: workflowID,
	}, approvalPolicy, s.ToolPolicy)
	if err != nil {
		return ApprovalEvaluation{}, err
	}
	if forceApproval && decision.Outcome == PolicyDecisionAllow {
		decision.Outcome = PolicyDecisionRequireApproval
		decision.Reason = "workflow risk assessment explicitly flagged approval"
		audit.Outcome = string(PolicyDecisionRequireApproval)
		audit.Reason = decision.Reason
	}
	return ApprovalEvaluation{
		RiskAssessment: risk,
		Decision:       &decision,
		Audit:          &audit,
	}, nil
}

func (s ApprovalService) EvaluateReport(
	workflowID string,
	actor string,
	audience string,
	containsPII bool,
) (ReportEvaluation, error) {
	engine := s.policyEngine()
	decision, audit, err := engine.EvaluateReport(ReportRequest{
		Actor:         actor,
		Audience:      audience,
		ContainsPII:   containsPII,
		CorrelationID: workflowID,
	}, s.reportPolicy())
	if err != nil {
		return ReportEvaluation{}, err
	}
	return ReportEvaluation{Decision: decision, Audit: audit}, nil
}

func (s ApprovalService) classifier() RiskClassifier {
	if s.Classifier != nil {
		return s.Classifier
	}
	return DefaultRiskClassifier{}
}

func (s ApprovalService) decider() ApprovalDecider {
	if s.Decider.PolicyEngine != nil {
		return s.Decider
	}
	return ApprovalDecider{PolicyEngine: s.policyEngine()}
}

func (s ApprovalService) policyEngine() PolicyEngine {
	if s.PolicyEngine != nil {
		return s.PolicyEngine
	}
	return StaticPolicyEngine{}
}

func (s ApprovalService) approvalPolicy() ApprovalPolicy {
	if s.ApprovalPolicy.Name != "" {
		return s.ApprovalPolicy
	}
	return ApprovalPolicy{
		Name:          "default-approval-policy",
		MinRiskLevel:  ActionRiskHigh,
		RequiredRoles: []string{"operator"},
		AutoApprove:   false,
	}
}

func (s ApprovalService) reportPolicy() ReportDisclosurePolicy {
	if s.ReportPolicy.Audience != "" {
		return s.ReportPolicy
	}
	return ReportDisclosurePolicy{Audience: "user", AllowPII: false}
}

type MemoryWriteGateService struct {
	PolicyEngine  PolicyEngine
	Policy        MemoryWritePolicy
	CorrelationID string
}

func (s MemoryWriteGateService) AllowWrite(_ context.Context, record memory.MemoryRecord) error {
	engine := s.PolicyEngine
	if engine == nil {
		engine = StaticPolicyEngine{}
	}
	policy := s.Policy
	if policy.MinConfidence == 0 && !policy.RequireEvidence && len(policy.AllowKinds) == 0 {
		policy = MemoryWritePolicy{
			MinConfidence: 0.7,
			AllowKinds: []memory.MemoryKind{
				memory.MemoryKindEpisodic,
				memory.MemoryKindSemantic,
				memory.MemoryKindProcedural,
				memory.MemoryKindPolicy,
			},
		}
	}
	correlationID := s.CorrelationID
	if correlationID == "" {
		correlationID = fmt.Sprintf("memory-gate-%d", time.Now().UTC().UnixNano())
	}
	decision, _, err := engine.EvaluateMemoryWrite(record, policy, correlationID)
	if err != nil {
		return err
	}
	if decision.Outcome != PolicyDecisionAllow {
		return fmt.Errorf("%s", decision.Reason)
	}
	return nil
}
