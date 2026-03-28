package governance

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/memory"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/state"
)

func TestPolicySchemaRoundTrip(t *testing.T) {
	policy := ApprovalPolicy{
		Name:          "high-risk-approval",
		MinRiskLevel:  ActionRiskHigh,
		RequiredRoles: []string{"operator", "analyst"},
		AutoApprove:   false,
	}
	data, err := json.Marshal(policy)
	if err != nil {
		t.Fatalf("marshal policy: %v", err)
	}
	var decoded ApprovalPolicy
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal policy: %v", err)
	}
	if err := decoded.Validate(); err != nil {
		t.Fatalf("decoded policy should validate: %v", err)
	}
}

func TestHighRiskActionRequiresApproval(t *testing.T) {
	engine := StaticPolicyEngine{}
	decision, audit, err := engine.EvaluateAction(
		ActionRequest{
			Actor:         "planner-agent",
			ActorRoles:    []string{"analyst"},
			Action:        "single_stock_recommendation",
			Resource:      "AAPL",
			RiskLevel:     ActionRiskHigh,
			CorrelationID: "corr-001",
		},
		ApprovalPolicy{Name: "high-risk", MinRiskLevel: ActionRiskHigh, RequiredRoles: []string{"operator"}, AutoApprove: false},
		&ToolExecutionPolicy{ToolName: "advisor", AllowedRoles: []string{"analyst"}, MaxCallsPerTask: 1, RequiresApprovalAbove: ActionRiskHigh},
	)
	if err != nil {
		t.Fatalf("evaluate action: %v", err)
	}
	if decision.Outcome != PolicyDecisionRequireApproval {
		t.Fatalf("expected require approval, got %+v", decision)
	}
	if audit.Outcome != string(PolicyDecisionRequireApproval) {
		t.Fatalf("expected audit outcome to track decision")
	}
}

func TestLowConfidenceMemoryWriteRejected(t *testing.T) {
	engine := StaticPolicyEngine{}
	now := time.Now().UTC()
	record := memory.MemoryRecord{
		ID:      "memory-low-confidence",
		Kind:    memory.MemoryKindSemantic,
		Summary: "Potential tax preference.",
		Facts:   []memory.MemoryFact{{Key: "tax_bracket", Value: "uncertain", EvidenceID: observation.EvidenceID("evidence-1")}},
		Source:  memory.MemorySource{TaskID: "task-1", Actor: "memory-steward"},
		Confidence: memory.MemoryConfidence{
			Score:     0.3,
			Rationale: "weak extraction",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	decision, _, err := engine.EvaluateMemoryWrite(record, MemoryWritePolicy{
		MinConfidence:   0.8,
		RequireEvidence: false,
		AllowKinds:      []memory.MemoryKind{memory.MemoryKindSemantic},
	}, "corr-memory-1")
	if err != nil {
		t.Fatalf("evaluate memory write: %v", err)
	}
	if decision.Outcome != PolicyDecisionDeny {
		t.Fatalf("expected memory write to be denied, got %+v", decision)
	}
}

func TestDefaultRiskClassifierClassifiesHighRisk(t *testing.T) {
	classifier := DefaultRiskClassifier{}
	assessment := classifier.Classify(state.FinancialWorldState{
		UserID: "user-1",
		LiabilityState: state.LiabilityState{
			DebtBurdenRatio:        0.4,
			MinimumPaymentPressure: 0.22,
		},
		RiskState: state.RiskState{
			OverallRisk: "high",
		},
	}, "single_stock_invest_recommendation")
	if assessment.Level != ActionRiskHigh {
		t.Fatalf("expected high risk classification, got %+v", assessment)
	}
}

func TestApprovalServiceEvaluatesActionAndReport(t *testing.T) {
	service := ApprovalService{
		Classifier:   DefaultRiskClassifier{},
		Decider:      ApprovalDecider{PolicyEngine: StaticPolicyEngine{}},
		PolicyEngine: StaticPolicyEngine{},
		ApprovalPolicy: ApprovalPolicy{
			Name:          "high-risk-review",
			MinRiskLevel:  ActionRiskHigh,
			RequiredRoles: []string{"operator"},
			AutoApprove:   false,
		},
		ReportPolicy: ReportDisclosurePolicy{Audience: "user", AllowPII: false},
	}
	current := state.FinancialWorldState{
		UserID: "user-1",
		LiabilityState: state.LiabilityState{
			DebtBurdenRatio:        0.42,
			MinimumPaymentPressure: 0.21,
		},
		RiskState: state.RiskState{OverallRisk: "high"},
	}
	evaluation, err := service.EvaluateAction(current, "workflow-1", "monthly_review_report", "task-1", "governance_agent", []string{"analyst"}, false)
	if err != nil {
		t.Fatalf("evaluate action: %v", err)
	}
	if evaluation.Decision == nil || evaluation.Decision.Outcome != PolicyDecisionRequireApproval {
		t.Fatalf("expected require approval, got %+v", evaluation)
	}

	reportEval, err := service.EvaluateReport("workflow-1", "report_agent", "user", true)
	if err != nil {
		t.Fatalf("evaluate report: %v", err)
	}
	if reportEval.Decision.Outcome != PolicyDecisionRedact {
		t.Fatalf("expected redact decision for pii report")
	}
}
