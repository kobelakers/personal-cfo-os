package app

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/observability"
	runtimepkg "github.com/kobelakers/personal-cfo-os/internal/runtime"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/verification"
)

func TestPhase6BBehaviorInterventionHappyPath(t *testing.T) {
	env := openPhase6BTestEnv(t, filepath.Join(t.TempDir(), "memory.db"), "holdings_2026-03-safe.csv", time.Date(2026, 3, 30, 8, 0, 0, 0, time.UTC))
	result, err := env.RunBehaviorIntervention(t.Context(), "user-1", "请帮我做一次订阅清理复盘", state.FinancialWorldState{})
	if err != nil {
		t.Fatalf("run behavior intervention happy path: %v", err)
	}
	if err := env.Close(); err != nil {
		t.Fatalf("close phase 6b env: %v", err)
	}

	if result.Result.RuntimeState != runtimepkg.WorkflowStateCompleted {
		t.Fatalf("expected completed behavior intervention, got %q", result.Result.RuntimeState)
	}
	if len(result.Result.BlockResults) != 1 || result.Result.BlockResults[0].Behavior == nil {
		t.Fatalf("expected behavior block result in main chain, got %+v", result.Result.BlockResults)
	}
	block := result.Result.BlockResults[0].Behavior
	if block.SelectedSkill.Family != "subscription_cleanup" || block.SelectedSkill.RecipeID != "subscription_cleanup.v1" {
		t.Fatalf("expected subscription cleanup skill selection, got %+v", block.SelectedSkill)
	}
	if len(result.Result.Report.Recommendations) == 0 || result.Result.Report.SelectedSkillFamily == "" {
		t.Fatalf("expected behavior report to carry recommendations and selected skill, got %+v", result.Result.Report)
	}
	if len(observability.BuildDebugSummaryFromTrace(result.Result.WorkflowID, result.Trace, string(result.Result.RuntimeState)).SkillSummary) == 0 {
		t.Fatalf("expected replay/debug surface to carry skill summary, got %+v", result.Trace)
	}
}

func TestPhase6BProceduralMemoryInfluencesSkillSelection(t *testing.T) {
	workDir := t.TempDir()
	memoryDB := filepath.Join(workDir, "memory.db")
	firstEnv := openPhase6BTestEnv(t, memoryDB, "holdings_2026-03-safe.csv", time.Date(2026, 3, 30, 8, 0, 0, 0, time.UTC))
	first, err := firstEnv.RunBehaviorIntervention(t.Context(), "user-1", "请帮我做一次消费护栏复盘", state.FinancialWorldState{})
	if err != nil {
		t.Fatalf("run first guardrail behavior intervention: %v", err)
	}
	if err := firstEnv.Close(); err != nil {
		t.Fatalf("close first phase 6b env: %v", err)
	}
	secondEnv := openPhase6BTestEnv(t, memoryDB, "holdings_2026-03-safe.csv", time.Date(2026, 3, 30, 8, 0, 1, 0, time.UTC))
	second, err := secondEnv.RunBehaviorIntervention(t.Context(), "user-1", "请再做一次消费护栏复盘", state.FinancialWorldState{})
	if err != nil {
		t.Fatalf("run second guardrail behavior intervention: %v", err)
	}
	comparison, err := secondEnv.ReplayQuery.Compare(t.Context(),
		observability.ReplayQuery{WorkflowID: first.Result.WorkflowID},
		observability.ReplayQuery{WorkflowID: second.Result.WorkflowID},
	)
	if err != nil {
		t.Fatalf("compare behavior intervention replay views: %v", err)
	}
	if err := secondEnv.Close(); err != nil {
		t.Fatalf("close second phase 6b env: %v", err)
	}

	if first.Result.Report.SelectedRecipeID != "soft_nudge.v1" {
		t.Fatalf("expected first run to start with soft_nudge.v1, got %q", first.Result.Report.SelectedRecipeID)
	}
	if second.Result.Report.SelectedRecipeID != "budget_guardrail.v1" {
		t.Fatalf("expected second run to escalate to budget_guardrail.v1, got %q", second.Result.Report.SelectedRecipeID)
	}
	if second.Result.SkillExecution == nil || len(second.Result.SkillExecution.ProducedMemoryRefs) == 0 {
		t.Fatalf("expected skill execution record to capture produced memory refs, got %+v", second.Result.SkillExecution)
	}
	if !comparisonHasCategory(comparison, "skill") {
		t.Fatalf("expected replay comparison to expose changed skill selection, got %+v", comparison)
	}
	if !comparisonContains(comparison, "procedural memory") && !comparisonContains(comparison, "budget_guardrail.v1") {
		t.Fatalf("expected comparison details to explain procedural memory influence, got %+v", comparison)
	}
}

func TestPhase6BBehaviorInterventionWaitingApproval(t *testing.T) {
	workDir := t.TempDir()
	memoryDB := filepath.Join(workDir, "memory.db")
	seedEnv := openPhase6BTestEnv(t, memoryDB, "holdings_2026-03-safe.csv", time.Date(2026, 3, 30, 8, 0, 0, 0, time.UTC))
	seed, err := seedEnv.RunBehaviorIntervention(t.Context(), "user-1", "请帮我做一次消费护栏复盘", state.FinancialWorldState{})
	if err != nil {
		t.Fatalf("seed behavior intervention procedural memory: %v", err)
	}
	if err := seedEnv.Close(); err != nil {
		t.Fatalf("close seed phase 6b env: %v", err)
	}
	approvalEnv := openPhase6BTestEnv(t, memoryDB, "holdings_2026-03-safe.csv", time.Date(2026, 3, 30, 8, 0, 1, 0, time.UTC))
	result, err := approvalEnv.RunBehaviorIntervention(t.Context(), "user-1", "请做一次 hard_cap 消费护栏复盘", state.FinancialWorldState{})
	if err != nil {
		t.Fatalf("run hard cap behavior intervention: %v", err)
	}
	if err := approvalEnv.Close(); err != nil {
		t.Fatalf("close approval phase 6b env: %v", err)
	}

	if seed.Result.Report.SelectedRecipeID != "soft_nudge.v1" {
		t.Fatalf("expected seed guardrail run to use soft_nudge.v1, got %q", seed.Result.Report.SelectedRecipeID)
	}
	if result.Result.RuntimeState != runtimepkg.WorkflowStateWaitingApproval {
		t.Fatalf("expected waiting_approval runtime state, got %q", result.Result.RuntimeState)
	}
	if result.Result.Report.SelectedRecipeID != "hard_cap.v1" {
		t.Fatalf("expected hard_cap.v1 recipe, got %q", result.Result.Report.SelectedRecipeID)
	}
	if result.Result.PendingApproval == nil || result.Result.ApprovalDecision == nil || result.Result.ApprovalDecision.Outcome != "require_approval" {
		t.Fatalf("expected approval metadata for hard cap guardrail, got pending=%+v decision=%+v", result.Result.PendingApproval, result.Result.ApprovalDecision)
	}
	if len(result.Result.Artifacts) != 0 {
		t.Fatalf("expected no finalized artifacts before approval, got %+v", result.Result.Artifacts)
	}
}

func TestPhase6BBehaviorInterventionTrustFailureTransitionsToFailed(t *testing.T) {
	env := openPhase6BTestEnvWithVerificationOverride(
		t,
		filepath.Join(t.TempDir(), "memory.db"),
		"holdings_2026-03-safe.csv",
		time.Date(2026, 3, 30, 8, 0, 0, 0, time.UTC),
		func(base verification.Pipeline) verification.Pipeline {
			base.GroundingValidator = forcedTrustFailureValidator{
				validator: "forced_behavior_grounding_failure",
				code:      "forced_behavior_trust_failure",
				message:   "test-only grounding failure for behavior intervention workflow integration",
			}
			return base
		},
	)
	result, err := env.RunBehaviorIntervention(t.Context(), "user-1", "请帮我做一次订阅清理复盘", state.FinancialWorldState{})
	if err != nil {
		t.Fatalf("run behavior intervention trust failure path: %v", err)
	}
	if err := env.Close(); err != nil {
		t.Fatalf("close phase 6b env: %v", err)
	}

	if result.Result.RuntimeState != runtimepkg.WorkflowStateFailed {
		t.Fatalf("expected behavior intervention trust failure to end as failed, got %q", result.Result.RuntimeState)
	}
	if !verification.HasTrustFailure(result.Result.Verification) {
		t.Fatalf("expected trust failure verification results, got %+v", result.Result.Verification)
	}
	if len(result.Result.Artifacts) != 0 {
		t.Fatalf("expected no success artifacts on trust failure, got %+v", result.Result.Artifacts)
	}
	if !traceContainsFailureCategory(result.Trace.Events, runtimepkg.FailureCategoryTrustValidation) {
		t.Fatalf("expected trace to record trust validation failure, got %+v", result.Trace.Events)
	}
}

func openPhase6BTestEnv(t *testing.T, memoryDB string, holdingsFixture string, now time.Time) *Phase6BEnvironment {
	t.Helper()
	runtimeDB := filepath.Join(filepath.Dir(memoryDB), "runtime.db")
	env, err := OpenPhase6BEnvironment(Phase6BOptions{
		FixtureDir:      monthlyReview5BFixtureDir(),
		HoldingsFixture: holdingsFixture,
		MemoryDBPath:    memoryDB,
		RuntimeDBPath:   runtimeDB,
		EmbeddingModel:  "mock-embedding-model",
		Now:             func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("open phase 6b environment: %v", err)
	}
	return env
}

func openPhase6BTestEnvWithVerificationOverride(
	t *testing.T,
	memoryDB string,
	holdingsFixture string,
	now time.Time,
	override func(base verification.Pipeline) verification.Pipeline,
) *Phase6BEnvironment {
	t.Helper()
	runtimeDB := filepath.Join(filepath.Dir(memoryDB), "runtime.db")
	env, err := OpenPhase6BEnvironment(Phase6BOptions{
		FixtureDir:      monthlyReview5BFixtureDir(),
		HoldingsFixture: holdingsFixture,
		MemoryDBPath:    memoryDB,
		RuntimeDBPath:   runtimeDB,
		EmbeddingModel:  "mock-embedding-model",
		Now:             func() time.Time { return now },
		VerificationPipelineOverride: override,
	})
	if err != nil {
		t.Fatalf("open phase 6b environment with verification override: %v", err)
	}
	return env
}

func comparisonContains(comparison observability.ReplayComparison, needle string) bool {
	for _, diff := range comparison.Diffs {
		for _, detail := range diff.Details {
			if strings.Contains(detail, needle) {
				return true
			}
		}
		for _, item := range diff.Left {
			if strings.Contains(item, needle) {
				return true
			}
		}
		for _, item := range diff.Right {
			if strings.Contains(item, needle) {
				return true
			}
		}
	}
	return false
}

func comparisonHasCategory(comparison observability.ReplayComparison, category string) bool {
	for _, diff := range comparison.Diffs {
		if diff.Category == category {
			return true
		}
	}
	return false
}
