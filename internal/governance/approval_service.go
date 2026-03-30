package governance

import (
	"context"
	"fmt"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/analysis"
	"github.com/kobelakers/personal-cfo-os/internal/memory"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
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

type ApprovalEvaluationInput struct {
	CurrentState     state.FinancialWorldState `json:"current_state"`
	WorkflowID       string                    `json:"workflow_id"`
	Action           string                    `json:"action"`
	Resource         string                    `json:"resource"`
	Actor            string                    `json:"actor"`
	ActorRoles       []string                  `json:"actor_roles,omitempty"`
	ForceApproval    bool                      `json:"force_approval"`
	Recommendations  []analysis.Recommendation `json:"recommendations,omitempty"`
	RiskFlags        []analysis.RiskFlag       `json:"risk_flags,omitempty"`
	ApprovalRequired bool                      `json:"approval_required"`
	ApprovalReason   string                    `json:"approval_reason,omitempty"`
	PolicyRuleRefs   []string                  `json:"policy_rule_refs,omitempty"`
	DisclosureReady  bool                      `json:"disclosure_ready"`
}

type ApprovalService struct {
	Classifier     RiskClassifier
	Decider        ApprovalDecider
	PolicyEngine   PolicyEngine
	ApprovalPolicy ApprovalPolicy
	ToolPolicy     *ToolExecutionPolicy
	ReportPolicy   ReportDisclosurePolicy
}

func (s ApprovalService) EvaluateAction(input ApprovalEvaluationInput) (ApprovalEvaluation, error) {
	classifier := s.classifier()
	risk := classifier.Classify(input.CurrentState, input.Action)
	risk = mergeRecommendationRisk(risk, input.Recommendations)
	approvalPolicy := s.approvalPolicy()
	decision, audit, err := s.decider().Decide(ActionRequest{
		Actor:            input.Actor,
		ActorRoles:       input.ActorRoles,
		Action:           input.Action,
		Resource:         input.Resource,
		RiskLevel:        risk.Level,
		Recommendations:  append([]analysis.Recommendation{}, input.Recommendations...),
		RiskFlags:        append([]analysis.RiskFlag{}, input.RiskFlags...),
		ApprovalRequired: input.ApprovalRequired,
		ApprovalReason:   input.ApprovalReason,
		PolicyRuleRefs:   append([]string{}, input.PolicyRuleRefs...),
		DisclosureReady:  input.DisclosureReady,
		CorrelationID:    input.WorkflowID,
	}, approvalPolicy, s.ToolPolicy)
	if err != nil {
		return ApprovalEvaluation{}, err
	}
	if input.ForceApproval && decision.Outcome == PolicyDecisionAllow {
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

func mergeRecommendationRisk(base RiskAssessment, recommendations []analysis.Recommendation) RiskAssessment {
	level := base.Level
	reasons := append([]string{}, base.Reasons...)
	for _, recommendation := range recommendations {
		mapped := recommendationRiskLevel(recommendation.RiskLevel)
		if compareRisk(mapped, level) > 0 {
			level = mapped
		}
	}
	if len(recommendations) > 0 {
		reasons = append(reasons, "recommendation contract supplied explicit risk semantics")
	}
	return RiskAssessment{Level: level, Reasons: uniqueStrings(reasons)}
}

func recommendationRiskLevel(level taskspec.RiskLevel) ActionRiskLevel {
	switch level {
	case "critical":
		return ActionRiskCritical
	case "high":
		return ActionRiskHigh
	case "medium":
		return ActionRiskMedium
	default:
		return ActionRiskLow
	}
}

func uniqueStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
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
