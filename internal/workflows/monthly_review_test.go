package workflows

import (
	"strings"
	"testing"

	"github.com/kobelakers/personal-cfo-os/internal/governance"
	"github.com/kobelakers/personal-cfo-os/internal/memory"
	"github.com/kobelakers/personal-cfo-os/internal/observability"
	runtimepkg "github.com/kobelakers/personal-cfo-os/internal/runtime"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/verification"
)

const safeHoldingsCSV = `user_id,account_id,snapshot_at,asset_class,symbol,market_value_cents,target_allocation
user-1,broker-1,2026-03-28T00:00:00Z,equity,VT,500000,0.50
user-1,broker-1,2026-03-28T00:00:00Z,bond,BND,300000,0.30
user-1,broker-1,2026-03-28T00:00:00Z,cash,CASH,200000,0.20
`

func TestMonthlyReviewWorkflowHappyPath(t *testing.T) {
	deps := buildPhase2Deps(t, safeHoldingsCSV, true, true)
	workflow := buildMonthlyReviewWorkflow(t, deps)

	result, err := workflow.Run(t.Context(), "user-1", "请帮我做一份月度财务复盘", stateZero())
	if err != nil {
		t.Fatalf("run monthly review: %v", err)
	}
	if result.Report.Summary == "" {
		t.Fatalf("expected non-empty report summary")
	}
	if len(result.BlockResults) != 2 {
		t.Fatalf("expected two domain block results, got %+v", result.BlockResults)
	}
	if result.RuntimeState != runtimepkg.WorkflowStateCompleted {
		t.Fatalf("expected completed runtime state, got %q", result.RuntimeState)
	}
	if len(result.GeneratedMemories) == 0 {
		t.Fatalf("expected generated memories")
	}
	if !agentRecipientSeen(deps.AgentTrace.Records(), "planner_agent") ||
		!agentRecipientSeen(deps.AgentTrace.Records(), "memory_steward") ||
		!agentRecipientSeen(deps.AgentTrace.Records(), "cashflow_agent") ||
		!agentRecipientSeen(deps.AgentTrace.Records(), "debt_agent") ||
		!agentRecipientSeen(deps.AgentTrace.Records(), "report_agent") ||
		!agentRecipientSeen(deps.AgentTrace.Records(), "verification_agent") ||
		!agentRecipientSeen(deps.AgentTrace.Records(), "governance_agent") {
		t.Fatalf("expected workflow to dispatch all system agents, got %+v", deps.AgentTrace.Records())
	}
	if result.CoverageReport.CoverageRatio < 1 {
		t.Fatalf("expected full coverage, got %.2f", result.CoverageReport.CoverageRatio)
	}
	for _, item := range result.Verification {
		if item.Status != verification.VerificationStatusPass {
			t.Fatalf("expected all verification results to pass, got %+v", result.Verification)
		}
	}
}

func TestMonthlyReviewWorkflowMissingEvidenceTriggersReplan(t *testing.T) {
	deps := buildPhase2Deps(t, safeHoldingsCSV, false, false)
	deps.LedgerAdapter.Holdings = nil
	workflow := buildMonthlyReviewWorkflow(t, deps)

	result, err := workflow.Run(t.Context(), "user-1", "请帮我做一份月度财务复盘", stateZero())
	if err != nil {
		t.Fatalf("run monthly review with missing evidence: %v", err)
	}
	if result.RuntimeState != runtimepkg.WorkflowStateReplanning {
		t.Fatalf("expected workflow to enter replanning, got %q", result.RuntimeState)
	}
	if result.CoverageReport.CoverageRatio >= 1 {
		t.Fatalf("expected incomplete coverage, got %.2f", result.CoverageReport.CoverageRatio)
	}
	if !hasVerificationStatus(result.Verification, verification.VerificationStatusNeedsReplan) {
		t.Fatalf("expected needs_replan verification result, got %+v", result.Verification)
	}
}

func TestMonthlyReviewWorkflowHighRiskRequiresApproval(t *testing.T) {
	deps := buildPhase2Deps(t, string(readWorkflowFixture(t, "holdings_2026-03.csv")), true, true)
	workflow := buildMonthlyReviewWorkflow(t, deps)

	result, err := workflow.Run(t.Context(), "user-1", "请帮我做一份月度财务复盘", stateZero())
	if err != nil {
		t.Fatalf("run high-risk monthly review: %v", err)
	}
	if result.ApprovalDecision == nil || result.ApprovalDecision.Outcome != governance.PolicyDecisionRequireApproval {
		t.Fatalf("expected approval decision to require approval, got %+v", result.ApprovalDecision)
	}
	if result.RuntimeState != runtimepkg.WorkflowStateWaitingApproval {
		t.Fatalf("expected waiting approval state, got %q", result.RuntimeState)
	}
}

func TestMonthlyReviewWorkflowGovernanceDeniesInvalidMemoryWrite(t *testing.T) {
	deps := buildPhase2Deps(t, safeHoldingsCSV, true, true)
	workflow := buildMonthlyReviewWorkflow(t, deps)
	workflow.SystemSteps = buildSystemStepBus(t, deps, governance.MemoryWritePolicy{
		MinConfidence:   0.95,
		RequireEvidence: true,
		AllowKinds:      []memory.MemoryKind{memory.MemoryKindSemantic},
	})

	_, err := workflow.Run(t.Context(), "user-1", "请帮我做一份月度财务复盘", stateZero())
	if err == nil {
		t.Fatalf("expected memory write policy to deny workflow")
	}
	if !strings.Contains(err.Error(), "memory write denied") {
		t.Fatalf("expected governance denial error, got %v", err)
	}
}

func stateZero() state.FinancialWorldState {
	return state.FinancialWorldState{UserID: "user-1"}
}

func hasVerificationStatus(results []verification.VerificationResult, status verification.VerificationStatus) bool {
	for _, result := range results {
		if result.Status == status {
			return true
		}
	}
	return false
}

func agentRecipientSeen(records []observability.AgentExecutionRecord, recipient string) bool {
	for _, record := range records {
		if record.Recipient == recipient && record.Lifecycle == observability.AgentLifecycleCompleted {
			return true
		}
	}
	return false
}
