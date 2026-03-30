package app

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/memory"
	"github.com/kobelakers/personal-cfo-os/internal/model"
	runtimepkg "github.com/kobelakers/personal-cfo-os/internal/runtime"
	"github.com/kobelakers/personal-cfo-os/internal/state"
)

func TestPhase5DMonthlyReviewTrustPath(t *testing.T) {
	env := openPhase5DTestEnv(t, filepath.Join(t.TempDir(), "memory.db"), "holdings_2026-03-safe.csv", time.Date(2026, 3, 30, 8, 0, 0, 0, time.UTC))
	result, err := env.RunMonthlyReview(t.Context(), "user-1", "请帮我做一份月度财务复盘", state.FinancialWorldState{})
	if err != nil {
		t.Fatalf("run monthly review 5d: %v", err)
	}
	if err := env.Close(); err != nil {
		t.Fatalf("close phase 5d env: %v", err)
	}

	if result.Result.RuntimeState != runtimepkg.WorkflowStateCompleted {
		t.Fatalf("expected completed monthly review runtime state, got %q with verification=%+v", result.Result.RuntimeState, result.Result.Verification)
	}
	if len(result.Result.Report.Recommendations) == 0 || len(result.Result.Report.RiskFlags) == 0 {
		t.Fatalf("expected grounded recommendations and risk flags, got %+v", result.Result.Report)
	}
	if len(result.Result.Report.MetricRecords) == 0 || len(result.Result.Report.GroundingRefs) == 0 || len(result.Result.Report.Caveats) == 0 {
		t.Fatalf("expected trust fields in final report, got %+v", result.Result.Report)
	}
	if len(result.Trace.FinanceMetrics) == 0 || len(result.Trace.GroundingVerdicts) == 0 || len(result.Trace.NumericValidationVerdicts) == 0 || len(result.Trace.BusinessRuleVerdicts) == 0 {
		t.Fatalf("expected trust trace bundle, got %+v", result.Trace)
	}
}

func TestPhase5DDebtVsInvestDeterministicWaitingApproval(t *testing.T) {
	env := openPhase5DTestEnv(t, filepath.Join(t.TempDir(), "memory.db"), "holdings_2026-03-safe.csv", time.Date(2026, 3, 30, 8, 0, 0, 0, time.UTC))
	result, err := env.RunDebtVsInvest(t.Context(), "user-1", "提前还贷还是继续投资更合适", state.FinancialWorldState{})
	if err != nil {
		t.Fatalf("run debt-vs-invest 5d: %v", err)
	}
	if result.Result.RuntimeState != runtimepkg.WorkflowStateWaitingApproval {
		t.Fatalf("expected deterministic waiting_approval path, got %q with decision=%+v", result.Result.RuntimeState, result.Result.ApprovalDecision)
	}
	if result.Result.ApprovalDecision == nil || result.Result.ApprovalDecision.Outcome != "require_approval" {
		t.Fatalf("expected approval decision, got %+v", result.Result.ApprovalDecision)
	}
	if result.Result.PendingApproval == nil || result.Result.Checkpoint == nil || result.Result.ResumeToken == nil {
		t.Fatalf("expected approval resume anchors, got pending=%+v checkpoint=%+v token=%+v", result.Result.PendingApproval, result.Result.Checkpoint, result.Result.ResumeToken)
	}
	if len(result.Trace.PolicyRuleHits) == 0 || len(result.Trace.ApprovalTriggers) == 0 {
		t.Fatalf("expected approval policy trace, got %+v", result.Trace)
	}

	resumed, err := env.ResumeDebtVsInvestAfterApproval(t.Context(), result.Result)
	if err != nil {
		t.Fatalf("resume debt-vs-invest after approval: %v", err)
	}
	if resumed.Result.RuntimeState != runtimepkg.WorkflowStateCompleted {
		t.Fatalf("expected resumed debt-vs-invest to complete, got %q", resumed.Result.RuntimeState)
	}
	if len(resumed.Result.Artifacts) == 0 || resumed.Result.Report.Conclusion == "" {
		t.Fatalf("expected finalized debt decision artifact after approval, got %+v", resumed.Result)
	}
	if err := env.Close(); err != nil {
		t.Fatalf("close phase 5d env: %v", err)
	}
}

func openPhase5DTestEnv(t *testing.T, memoryDB string, holdingsFixture string, now time.Time) *Phase5DEnvironment {
	t.Helper()
	env, err := OpenPhase5DEnvironment(Phase5DOptions{
		FixtureDir:      monthlyReview5BFixtureDir(),
		HoldingsFixture: holdingsFixture,
		MemoryDBPath:    memoryDB,
		EmbeddingModel:  "mock-embedding-model",
		Now:             func() time.Time { return now },
		ChatModelFactory: func(callRecorder model.CallRecorder, usageRecorder model.UsageRecorder) model.ChatModel {
			return NewMockMonthlyReviewChatModelWithTrace(callRecorder, usageRecorder)
		},
		EmbeddingProviderFactory: func(callRecorder memory.EmbeddingCallRecorder, usageRecorder memory.EmbeddingUsageRecorder) memory.EmbeddingProvider {
			return NewMockMonthlyReviewEmbeddingProvider(callRecorder, usageRecorder)
		},
	})
	if err != nil {
		t.Fatalf("open phase 5d environment: %v", err)
	}
	return env
}
