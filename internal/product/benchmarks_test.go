package product

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/eval"
)

func TestBenchmarkSurfaceServiceLoadsSummariesAndExportsMarkdown(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	run := sampleEvalRun("phase6b-default", "phase6b-run", 4, 4, 0, 780)
	writeEvalRunSample(t, dir, "phase6b_eval_default_corpus.json", run)

	service := NewBenchmarkSurfaceService(dir)

	list, err := service.ListRuns()
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 benchmark summary, got %d", len(list))
	}
	if list[0].CorpusID != "phase6b-default" {
		t.Fatalf("expected phase6b-default corpus, got %s", list[0].CorpusID)
	}

	detail, err := service.GetRun("phase6b_eval_default_corpus")
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if detail.Summary.PassedCount != 4 {
		t.Fatalf("expected 4 passed scenarios, got %d", detail.Summary.PassedCount)
	}

	markdown, err := service.ExportRunMarkdown("phase6b_eval_default_corpus")
	if err != nil {
		t.Fatalf("export markdown: %v", err)
	}
	if markdown == "" {
		t.Fatal("expected markdown export to be non-empty")
	}
	if want := "# Benchmark phase6b-default"; !strings.Contains(markdown, want) {
		t.Fatalf("unexpected markdown header: %s", markdown)
	}
}

func TestBenchmarkSurfaceServiceCompare(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	left := sampleEvalRun("phase6a-default", "phase6a-run", 11, 11, 0, 640)
	right := sampleEvalRun("phase6b-default", "phase6b-run", 4, 4, 0, 780)
	writeEvalRunSample(t, dir, "phase6a_eval_default_corpus.json", left)
	writeEvalRunSample(t, dir, "phase6b_eval_default_corpus.json", right)

	service := NewBenchmarkSurfaceService(dir)
	view, err := service.Compare("phase6b_eval_default_corpus", "phase6a_eval_default_corpus")
	if err != nil {
		t.Fatalf("compare runs: %v", err)
	}
	if len(view.Summary) == 0 {
		t.Fatal("expected non-empty benchmark compare summary")
	}
	if view.Left.ID == view.Right.ID {
		t.Fatalf("expected left and right benchmark ids to differ, got %s", view.Left.ID)
	}
}

func TestBenchmarkSurfaceServiceMissingDirectoryReturnsEmptyCatalog(t *testing.T) {
	t.Parallel()

	service := NewBenchmarkSurfaceService(filepath.Join(t.TempDir(), "missing"))
	list, err := service.ListRuns()
	if err != nil {
		t.Fatalf("list runs on missing dir: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected empty benchmark catalog, got %d items", len(list))
	}
}

func writeEvalRunSample(t *testing.T, dir string, name string, run eval.EvalRun) {
	t.Helper()
	payload, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		t.Fatalf("marshal eval run: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), payload, 0o644); err != nil {
		t.Fatalf("write eval run sample: %v", err)
	}
}

func sampleEvalRun(corpusID string, runID string, scenarios int, passed int, failed int, tokens int) eval.EvalRun {
	startedAt := time.Date(2026, 3, 31, 8, 0, 0, 0, time.UTC)
	return eval.EvalRun{
		RunID:             runID,
		CorpusID:          corpusID,
		DeterministicOnly: true,
		StartedAt:         startedAt,
		CompletedAt:       startedAt,
		Results: []eval.EvalResult{
			{
				ScenarioID:           corpusID + "-scenario",
				Passed:               true,
				RuntimeState:         "completed",
				TokenUsage:           tokens,
				DurationMilliseconds: 42,
			},
		},
		Score: eval.EvalScore{
			ScenarioCount:              scenarios,
			PassedCount:                passed,
			FailedCount:                failed,
			AverageLatencyMilliseconds: 42,
			TotalTokenUsage:            tokens,
			ValidatorPassRate:          1,
			ApprovalFrequency:          0.25,
		},
		Summary: eval.EvalSummary{
			CorpusID:          corpusID,
			DeterministicOnly: true,
			PassedScenarios:   []string{corpusID + "-scenario"},
			SummaryLines:      []string{corpusID + " benchmark summary"},
		},
	}
}
