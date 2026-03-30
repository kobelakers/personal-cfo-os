package eval

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/observability"
	"github.com/kobelakers/personal-cfo-os/internal/runtime"
)

func TestDefaultScenarioCorpusRunsDeterministically(t *testing.T) {
	t.Parallel()

	harness := NewHarness(HarnessOptions{
		FixtureDir: filepath.Join("..", "..", "tests", "fixtures"),
		WorkDir:    t.TempDir(),
		Now: func() time.Time {
			return time.Date(2026, 3, 29, 8, 0, 0, 0, time.UTC)
		},
	})

	runResult, err := harness.RunCorpus(context.Background(), DefaultScenarioCorpus())
	if err != nil {
		t.Fatalf("run default scenario corpus: %v", err)
	}
	if !runResult.DeterministicOnly {
		t.Fatalf("expected default corpus to remain deterministic-only")
	}
	if runResult.Score.ScenarioCount != 11 {
		t.Fatalf("expected 11 canonical scenarios, got %d", runResult.Score.ScenarioCount)
	}
	if runResult.Score.FailedCount != 0 {
		t.Fatalf("expected deterministic corpus to pass, got %+v", runResult)
	}
}

func TestPhase6BScenarioCorpusRunsDeterministically(t *testing.T) {
	t.Parallel()

	harness := NewHarness(HarnessOptions{
		FixtureDir: filepath.Join("..", "..", "tests", "fixtures"),
		WorkDir:    t.TempDir(),
		Now: func() time.Time {
			return time.Date(2026, 3, 30, 8, 0, 0, 0, time.UTC)
		},
	})

	runResult, err := harness.RunCorpus(context.Background(), Phase6BDefaultScenarioCorpus())
	if err != nil {
		t.Fatalf("run phase 6b scenario corpus: %v", err)
	}
	if !runResult.DeterministicOnly {
		t.Fatalf("expected 6b corpus to remain deterministic-only")
	}
	if runResult.Score.ScenarioCount != 4 {
		t.Fatalf("expected 4 deterministic 6b scenarios, got %d", runResult.Score.ScenarioCount)
	}
	if runResult.Score.FailedCount != 0 {
		t.Fatalf("expected 6b corpus to pass, got %+v", runResult)
	}
}

func TestHarnessReportsRegressionFailures(t *testing.T) {
	t.Parallel()

	harness := NewHarness(HarnessOptions{
		WorkDir: t.TempDir(),
		Now: func() time.Time {
			return time.Date(2026, 3, 29, 9, 0, 0, 0, time.UTC)
		},
	})

	result, err := harness.RunScenario(context.Background(), ScenarioCase{
		ID:            "forced_failure",
		Category:      "unit",
		Description:   "force a regression failure for harness validation",
		Deterministic: true,
		Expectation: ScenarioExpectation{
			ExpectedFinalState: "completed",
			ExpectedScopeKind:  "workflow",
		},
		run: func(_ context.Context, _ ScenarioRunContext) (scenarioRunOutput, error) {
			return scenarioRunOutput{
				RuntimeState: runtime.WorkflowStateFailed,
				Replay: observability.ReplayView{
					Scope:   observability.ReplayScope{Kind: "workflow", ID: "workflow-forced"},
					Summary: observability.ReplaySummary{FinalState: "failed"},
				},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("run forced failure scenario: %v", err)
	}
	if result.Passed {
		t.Fatalf("expected forced failure scenario to fail regression checks")
	}
	if len(result.RegressionFailures) == 0 {
		t.Fatalf("expected regression failures to be recorded")
	}
}

func TestCompareRunsReportsScenarioChanges(t *testing.T) {
	t.Parallel()

	left := EvalRun{
		RunID: "left",
		Results: []EvalResult{
			{
				ScenarioID:   "monthly_review_happy_path",
				Passed:       true,
				RuntimeState: "completed",
				Scope:        observability.ReplayScope{Kind: "workflow", ID: "workflow-left"},
				Replay: observability.ReplayView{
					Summary: observability.ReplaySummary{
						PlanSummary: []string{"cashflow-review", "debt-review"},
					},
				},
			},
		},
	}
	right := EvalRun{
		RunID: "right",
		Results: []EvalResult{
			{
				ScenarioID:   "monthly_review_happy_path",
				Passed:       true,
				RuntimeState: "completed",
				Scope:        observability.ReplayScope{Kind: "workflow", ID: "workflow-right"},
				Replay: observability.ReplayView{
					Summary: observability.ReplaySummary{
						PlanSummary:   []string{"cashflow-review", "debt-review", "tax-review"},
						MemorySummary: []string{"selected=memory-a"},
					},
				},
			},
		},
	}

	diff := CompareRuns(left, right)
	if len(diff.Differences) == 0 {
		t.Fatalf("expected eval diff to detect scenario changes")
	}
}
