package workflows

import (
	"testing"

	"github.com/kobelakers/personal-cfo-os/internal/governance"
)

func TestDebtVsInvestWorkflowMVPPath(t *testing.T) {
	deps := buildPhase2Deps(t, safeHoldingsCSV, true, true)
	workflow := buildDebtWorkflow(t, deps)

	result, err := workflow.Run(t.Context(), "user-1", "提前还贷还是继续投资更合适", stateZero())
	if err != nil {
		t.Fatalf("run debt-vs-invest workflow: %v", err)
	}
	if result.Report.Conclusion == "" {
		t.Fatalf("expected evidence-backed conclusion")
	}
	if len(result.Verification) == 0 {
		t.Fatalf("expected verification results")
	}
	if result.RiskAssessment.Level == "" {
		t.Fatalf("expected risk assessment to be populated")
	}
	if result.ApprovalDecision == nil {
		t.Fatalf("expected approval decision")
	}
	if result.ApprovalDecision.Outcome != governance.PolicyDecisionAllow && result.ApprovalDecision.Outcome != governance.PolicyDecisionRequireApproval {
		t.Fatalf("unexpected approval outcome: %+v", result.ApprovalDecision)
	}
	if !agentRecipientSeen(deps.AgentTrace.Records(), "planner_agent") ||
		!agentRecipientSeen(deps.AgentTrace.Records(), "memory_steward") ||
		!agentRecipientSeen(deps.AgentTrace.Records(), "report_agent") ||
		!agentRecipientSeen(deps.AgentTrace.Records(), "verification_agent") ||
		!agentRecipientSeen(deps.AgentTrace.Records(), "governance_agent") {
		t.Fatalf("expected debt workflow to dispatch all system agents, got %+v", deps.AgentTrace.Records())
	}
	records, err := deps.Store.List(t.Context())
	if err != nil {
		t.Fatalf("list written memories: %v", err)
	}
	if len(records) == 0 {
		t.Fatalf("expected debt workflow to write derived memories")
	}
}
