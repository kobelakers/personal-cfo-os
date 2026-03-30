package app

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	contextview "github.com/kobelakers/personal-cfo-os/internal/context"
	"github.com/kobelakers/personal-cfo-os/internal/memory"
	"github.com/kobelakers/personal-cfo-os/internal/model"
	"github.com/kobelakers/personal-cfo-os/internal/observability"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
	runtimepkg "github.com/kobelakers/personal-cfo-os/internal/runtime"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
	"github.com/kobelakers/personal-cfo-os/internal/verification"
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

func TestPhase5DMonthlyReviewTrustFailureTransitionsToFailed(t *testing.T) {
	env := openPhase5DTestEnvWithVerificationOverride(
		t,
		filepath.Join(t.TempDir(), "memory.db"),
		"holdings_2026-03-safe.csv",
		time.Date(2026, 3, 30, 8, 0, 0, 0, time.UTC),
		func(base verification.Pipeline) verification.Pipeline {
			base.GroundingValidator = forcedTrustFailureValidator{
				validator: "forced_monthly_review_grounding_failure",
				code:      "forced_monthly_review_trust_failure",
				message:   "test-only grounding failure for monthly review workflow integration",
			}
			return base
		},
	)
	result, err := env.RunMonthlyReview(t.Context(), "user-1", "请帮我做一份月度财务复盘", state.FinancialWorldState{})
	if err != nil {
		t.Fatalf("run monthly review trust failure path: %v", err)
	}
	if err := env.Close(); err != nil {
		t.Fatalf("close phase 5d env: %v", err)
	}

	if result.Result.RuntimeState != runtimepkg.WorkflowStateFailed {
		t.Fatalf("expected monthly review trust failure to end as failed, got %q", result.Result.RuntimeState)
	}
	if !verification.HasTrustFailure(result.Result.Verification) {
		t.Fatalf("expected trust failure verification results, got %+v", result.Result.Verification)
	}
	if len(result.Result.Artifacts) != 0 {
		t.Fatalf("expected no success artifacts on trust failure, got %+v", result.Result.Artifacts)
	}
	if len(result.Trace.GroundingVerdicts) == 0 || result.Trace.GroundingVerdicts[0].Status != verification.VerificationStatusFail {
		t.Fatalf("expected failed grounding verdicts in trace, got %+v", result.Trace.GroundingVerdicts)
	}
	if !traceContainsFailureCategory(result.Trace.Events, runtimepkg.FailureCategoryTrustValidation) {
		t.Fatalf("expected trace to record trust validation failure, got %+v", result.Trace.Events)
	}
}

func TestPhase5DDebtVsInvestTrustFailureTransitionsToFailed(t *testing.T) {
	env := openPhase5DTestEnvWithVerificationOverride(
		t,
		filepath.Join(t.TempDir(), "memory.db"),
		"holdings_2026-03-safe.csv",
		time.Date(2026, 3, 30, 8, 0, 0, 0, time.UTC),
		func(base verification.Pipeline) verification.Pipeline {
			base.GroundingValidator = forcedTrustFailureValidator{
				validator: "forced_debt_decision_grounding_failure",
				code:      "forced_debt_decision_trust_failure",
				message:   "test-only grounding failure for debt-vs-invest workflow integration",
			}
			return base
		},
	)
	result, err := env.RunDebtVsInvest(t.Context(), "user-1", "提前还贷还是继续投资更合适", state.FinancialWorldState{})
	if err != nil {
		t.Fatalf("run debt-vs-invest trust failure path: %v", err)
	}
	if err := env.Close(); err != nil {
		t.Fatalf("close phase 5d env: %v", err)
	}

	if result.Result.RuntimeState != runtimepkg.WorkflowStateFailed {
		t.Fatalf("expected debt-vs-invest trust failure to end as failed, got %q", result.Result.RuntimeState)
	}
	if !verification.HasTrustFailure(result.Result.Verification) {
		t.Fatalf("expected trust failure verification results, got %+v", result.Result.Verification)
	}
	if len(result.Result.Artifacts) != 0 {
		t.Fatalf("expected no success artifacts on trust failure, got %+v", result.Result.Artifacts)
	}
	if len(result.Trace.GroundingVerdicts) == 0 || result.Trace.GroundingVerdicts[0].Status != verification.VerificationStatusFail {
		t.Fatalf("expected failed grounding verdicts in trace, got %+v", result.Trace.GroundingVerdicts)
	}
	if !traceContainsFailureCategory(result.Trace.Events, runtimepkg.FailureCategoryTrustValidation) {
		t.Fatalf("expected trace to record trust validation failure, got %+v", result.Trace.Events)
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
	if len(result.Result.Artifacts) != 0 {
		t.Fatalf("expected no completed success artifacts before approval, got %+v", result.Result.Artifacts)
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

func TestPhase5DWorkflowReplayProjectionCompletedIsFresh(t *testing.T) {
	env := openPhase5DTestEnv(t, filepath.Join(t.TempDir(), "memory.db"), "holdings_2026-03-safe.csv", time.Date(2026, 3, 30, 8, 0, 0, 0, time.UTC))
	result, err := env.RunMonthlyReview(t.Context(), "user-1", "请帮我做一份月度财务复盘", state.FinancialWorldState{})
	if err != nil {
		t.Fatalf("run monthly review for workflow replay freshness: %v", err)
	}
	view, err := env.ReplayQuery.Query(t.Context(), observability.ReplayQuery{WorkflowID: result.Result.WorkflowID})
	if err != nil {
		t.Fatalf("query monthly review replay by workflow: %v", err)
	}
	if view.Degraded {
		t.Fatalf("expected fresh workflow replay projection after completed run, got %+v", view.DegradationReasons)
	}
	if view.Summary.FinalState != string(runtimepkg.WorkflowStateCompleted) {
		t.Fatalf("expected completed workflow replay summary, got %+v", view.Summary)
	}
	if err := env.Close(); err != nil {
		t.Fatalf("close phase 5d env: %v", err)
	}
}

func TestPhase5DWorkflowReplayProjectionRefreshesAcrossApprovalResume(t *testing.T) {
	env := openPhase5DTestEnv(t, filepath.Join(t.TempDir(), "memory.db"), "holdings_2026-03-safe.csv", time.Date(2026, 3, 30, 8, 0, 0, 0, time.UTC))
	initial, err := env.RunDebtVsInvest(t.Context(), "user-1", "提前还贷还是继续投资更合适", state.FinancialWorldState{})
	if err != nil {
		t.Fatalf("run debt-vs-invest waiting approval path: %v", err)
	}
	waitingView, err := env.ReplayQuery.Query(t.Context(), observability.ReplayQuery{WorkflowID: initial.Result.WorkflowID})
	if err != nil {
		t.Fatalf("query waiting approval workflow replay: %v", err)
	}
	if waitingView.Degraded {
		t.Fatalf("expected fresh waiting_approval workflow replay projection, got %+v", waitingView.DegradationReasons)
	}
	if waitingView.Summary.FinalState != string(runtimepkg.WorkflowStateWaitingApproval) {
		t.Fatalf("expected waiting_approval workflow replay summary, got %+v", waitingView.Summary)
	}

	resumed, err := env.ResumeDebtVsInvestAfterApproval(t.Context(), initial.Result)
	if err != nil {
		t.Fatalf("resume debt-vs-invest after approval: %v", err)
	}
	resumedView, err := env.ReplayQuery.Query(t.Context(), observability.ReplayQuery{WorkflowID: resumed.Result.WorkflowID})
	if err != nil {
		t.Fatalf("query resumed workflow replay: %v", err)
	}
	if resumedView.Degraded {
		t.Fatalf("expected resumed workflow replay projection to stay fresh, got %+v", resumedView.DegradationReasons)
	}
	if resumedView.Summary.FinalState != string(runtimepkg.WorkflowStateCompleted) {
		t.Fatalf("expected resumed workflow replay summary to refresh to completed, got %+v", resumedView.Summary)
	}
	if err := env.Close(); err != nil {
		t.Fatalf("close phase 5d env: %v", err)
	}
}

func TestPhase5DWorkflowReplayProjectionFailedIsFresh(t *testing.T) {
	env := openPhase5DTestEnvWithVerificationOverride(
		t,
		filepath.Join(t.TempDir(), "memory.db"),
		"holdings_2026-03-safe.csv",
		time.Date(2026, 3, 30, 8, 0, 0, 0, time.UTC),
		func(base verification.Pipeline) verification.Pipeline {
			base.GroundingValidator = forcedTrustFailureValidator{
				validator: "forced_monthly_review_grounding_failure",
				code:      "forced_monthly_review_trust_failure",
				message:   "test-only grounding failure for workflow replay freshness",
			}
			return base
		},
	)
	result, err := env.RunMonthlyReview(t.Context(), "user-1", "请帮我做一份月度财务复盘", state.FinancialWorldState{})
	if err != nil {
		t.Fatalf("run failed monthly review for workflow replay freshness: %v", err)
	}
	view, err := env.ReplayQuery.Query(t.Context(), observability.ReplayQuery{WorkflowID: result.Result.WorkflowID})
	if err != nil {
		t.Fatalf("query failed workflow replay: %v", err)
	}
	if view.Degraded {
		t.Fatalf("expected failed workflow replay projection to be fresh, got %+v", view.DegradationReasons)
	}
	if view.Summary.FinalState != string(runtimepkg.WorkflowStateFailed) {
		t.Fatalf("expected failed workflow replay summary, got %+v", view.Summary)
	}
	if len(view.FailureAttributions) == 0 {
		t.Fatalf("expected workflow replay failure attribution for failed path, got %+v", view)
	}
	if err := env.Close(); err != nil {
		t.Fatalf("close phase 5d env: %v", err)
	}
}

func TestPhase5DWorkflowReplayProjectionStaleReturnsDegradedPartialView(t *testing.T) {
	env := openPhase5DTestEnv(t, filepath.Join(t.TempDir(), "memory.db"), "holdings_2026-03-safe.csv", time.Date(2026, 3, 30, 8, 0, 0, 0, time.UTC))
	result, err := env.RunMonthlyReview(t.Context(), "user-1", "请帮我做一份月度财务复盘", state.FinancialWorldState{})
	if err != nil {
		t.Fatalf("run monthly review for stale workflow replay test: %v", err)
	}
	record, ok, err := env.RuntimeStores.WorkflowRuns.Load(result.Result.WorkflowID)
	if err != nil || !ok {
		t.Fatalf("load workflow run record for stale replay test: ok=%t err=%v", ok, err)
	}
	record.RuntimeState = runtimepkg.WorkflowStateFailed
	record.FailureCategory = runtimepkg.FailureCategoryTrustValidation
	record.FailureSummary = "workflow replay projection intentionally left stale"
	record.UpdatedAt = record.UpdatedAt.Add(2 * time.Hour)
	if err := env.RuntimeStores.WorkflowRuns.Save(record); err != nil {
		t.Fatalf("save stale workflow runtime truth: %v", err)
	}

	view, err := env.ReplayQuery.Query(t.Context(), observability.ReplayQuery{WorkflowID: result.Result.WorkflowID})
	if err != nil {
		t.Fatalf("query stale workflow replay: %v", err)
	}
	if !view.Degraded {
		t.Fatalf("expected stale workflow replay projection to degrade")
	}
	if view.Summary.FinalState != string(runtimepkg.WorkflowStateFailed) {
		t.Fatalf("expected authoritative failed state to win when projection is stale, got %+v", view.Summary)
	}
	if err := env.Close(); err != nil {
		t.Fatalf("close phase 5d env: %v", err)
	}
}

func openPhase5DTestEnv(t *testing.T, memoryDB string, holdingsFixture string, now time.Time) *Phase5DEnvironment {
	t.Helper()
	runtimeDB := filepath.Join(filepath.Dir(memoryDB), "runtime.db")
	env, err := OpenPhase5DEnvironment(Phase5DOptions{
		FixtureDir:      monthlyReview5BFixtureDir(),
		HoldingsFixture: holdingsFixture,
		MemoryDBPath:    memoryDB,
		RuntimeDBPath:   runtimeDB,
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

func openPhase5DTestEnvWithVerificationOverride(
	t *testing.T,
	memoryDB string,
	holdingsFixture string,
	now time.Time,
	override func(base verification.Pipeline) verification.Pipeline,
) *Phase5DEnvironment {
	t.Helper()
	runtimeDB := filepath.Join(filepath.Dir(memoryDB), "runtime.db")
	env, err := OpenPhase5DEnvironment(Phase5DOptions{
		FixtureDir:      monthlyReview5BFixtureDir(),
		HoldingsFixture: holdingsFixture,
		MemoryDBPath:    memoryDB,
		RuntimeDBPath:   runtimeDB,
		EmbeddingModel:  "mock-embedding-model",
		Now:             func() time.Time { return now },
		ChatModelFactory: func(callRecorder model.CallRecorder, usageRecorder model.UsageRecorder) model.ChatModel {
			return NewMockMonthlyReviewChatModelWithTrace(callRecorder, usageRecorder)
		},
		EmbeddingProviderFactory: func(callRecorder memory.EmbeddingCallRecorder, usageRecorder memory.EmbeddingUsageRecorder) memory.EmbeddingProvider {
			return NewMockMonthlyReviewEmbeddingProvider(callRecorder, usageRecorder)
		},
		VerificationPipelineOverride: override,
	})
	if err != nil {
		t.Fatalf("open phase 5d environment with verification override: %v", err)
	}
	return env
}

type forcedTrustFailureValidator struct {
	validator string
	code      string
	message   string
}

func (v forcedTrustFailureValidator) Validate(
	_ context.Context,
	spec taskspec.TaskSpec,
	_ state.FinancialWorldState,
	_ []observation.EvidenceRecord,
	_ []memory.MemoryRecord,
	_ contextview.BlockVerificationContext,
	_ any,
) ([]verification.VerificationResult, error) {
	now := time.Now().UTC()
	return []verification.VerificationResult{
		{
			Status:    verification.VerificationStatusFail,
			Scope:     verification.VerificationScopeFinal,
			Validator: v.validator,
			Message:   v.message,
			Category:  verification.ValidationCategoryGrounding,
			Severity:  string(verification.ValidationSeverityCritical),
			Diagnostics: []verification.ValidationDiagnostic{
				{
					Code:     v.code,
					Category: verification.ValidationCategoryGrounding,
					Severity: verification.ValidationSeverityCritical,
					Message:  v.message,
				},
			},
			EvidenceCoverage: verification.EvidenceCoverageReport{TaskID: spec.ID},
			CheckedAt:        now,
		},
	}, nil
}

func traceContainsFailureCategory(events []observability.LogEntry, category runtimepkg.FailureCategory) bool {
	for _, entry := range events {
		if entry.Category != "failure" {
			continue
		}
		if entry.Details["category"] == string(category) {
			return true
		}
	}
	return false
}
