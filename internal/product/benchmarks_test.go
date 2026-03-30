package product

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/eval"
	"github.com/kobelakers/personal-cfo-os/internal/reporting"
	"github.com/kobelakers/personal-cfo-os/internal/runtime"
)

func TestBenchmarkSurfaceServiceLoadsSampleAndArtifactRunsWithDedupe(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	artifacts := runtime.NewInMemoryArtifactMetadataStore()
	workflowRuns := runtime.NewInMemoryWorkflowRunStore()

	sample := sampleEvalRun("phase6b-default", "shared-run", 4, 4, 0, 780)
	writeEvalRunSample(t, dir, "phase6b_eval_default_corpus.json", sample)

	if err := workflowRuns.Save(runtime.WorkflowRunRecord{
		WorkflowID:   "workflow-benchmark-artifact",
		TaskID:       "task-benchmark-artifact",
		Intent:       "benchmark_seed",
		RuntimeState: runtime.WorkflowStateCompleted,
		StartedAt:    sample.StartedAt,
		UpdatedAt:    sample.CompletedAt,
		Summary:      "artifact benchmark seed",
	}); err != nil {
		t.Fatalf("save workflow run: %v", err)
	}
	writeEvalRunArtifact(t, artifacts, "workflow-benchmark-artifact", "artifact-benchmark-1", sampleEvalRun("phase7b-default", "artifact-only-run", 2, 2, 0, 320), "")
	writeEvalRunArtifact(t, artifacts, "workflow-benchmark-artifact", "artifact-benchmark-duplicate", sample, "")

	service := NewBenchmarkSurfaceService(BenchmarkSurfaceOptions{
		SampleDir:    dir,
		Artifacts:    artifacts,
		WorkflowRuns: workflowRuns,
	})

	list, err := service.ListRuns()
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 benchmark summaries after sample/artifact dedupe, got %d", len(list))
	}
	if list[0].Source != "artifact" && list[1].Source != "artifact" {
		t.Fatalf("expected one artifact-backed benchmark entry, got %+v", list)
	}
	if list[0].Source != "sample" && list[1].Source != "sample" {
		t.Fatalf("expected one sample-backed benchmark entry, got %+v", list)
	}
	for _, item := range list {
		if item.Source == "sample" && item.RunID == "shared-run" && strings.Contains(item.ID, "artifact-") {
			t.Fatalf("expected sample-backed shared-run to win dedupe, got %+v", item)
		}
	}
}

func TestBenchmarkSurfaceServiceCostSummaryUsesExplicitCostWhenPresent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	artifacts := runtime.NewInMemoryArtifactMetadataStore()
	workflowRuns := runtime.NewInMemoryWorkflowRunStore()
	now := time.Date(2026, 3, 31, 8, 0, 0, 0, time.UTC)

	if err := workflowRuns.Save(runtime.WorkflowRunRecord{
		WorkflowID:   "workflow-benchmark-artifact",
		TaskID:       "task-benchmark-artifact",
		Intent:       "benchmark_seed",
		RuntimeState: runtime.WorkflowStateCompleted,
		StartedAt:    now,
		UpdatedAt:    now,
		Summary:      "artifact benchmark seed",
	}); err != nil {
		t.Fatalf("save workflow run: %v", err)
	}
	run := sampleEvalRun("phase7b-default", "artifact-cost-run", 1, 1, 0, 240)
	writeEvalRunArtifact(t, artifacts, "workflow-benchmark-artifact", "artifact-benchmark-cost", run, `{"usd":0.03125,"precision":"recorded_exact","source":"artifact_payload"}`)

	service := NewBenchmarkSurfaceService(BenchmarkSurfaceOptions{
		SampleDir:    dir,
		Artifacts:    artifacts,
		WorkflowRuns: workflowRuns,
	})

	detail, err := service.GetRun("artifact-artifact-benchmark-cost")
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if detail.Summary.CostSummary.Precision != BenchmarkCostPrecisionRecordedExact {
		t.Fatalf("expected recorded_exact precision, got %+v", detail.Summary.CostSummary)
	}
	if detail.Summary.CostSummary.USD != 0.03125 {
		t.Fatalf("expected explicit artifact cost, got %+v", detail.Summary.CostSummary)
	}
}

func TestBenchmarkSurfaceServiceEstimatesCostFromTokensWhenCostMissing(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	run := sampleEvalRun("phase6a-default", "phase6a-run", 11, 11, 0, 9054)
	writeEvalRunSample(t, dir, "phase6a_eval_default_corpus.json", run)

	service := NewBenchmarkSurfaceService(BenchmarkSurfaceOptions{SampleDir: dir})
	detail, err := service.GetRun("phase6a_eval_default_corpus")
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if detail.Summary.CostSummary.Precision != BenchmarkCostPrecisionEstimatedFromTokens {
		t.Fatalf("expected estimated_from_tokens precision, got %+v", detail.Summary.CostSummary)
	}
	if detail.Summary.CostSummary.USD <= 0 {
		t.Fatalf("expected positive estimated cost, got %+v", detail.Summary.CostSummary)
	}
}

func TestBenchmarkSurfaceServiceLoadsSummariesAndExportsMarkdown(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	run := sampleEvalRun("phase6b-default", "phase6b-run", 4, 4, 0, 780)
	writeEvalRunSample(t, dir, "phase6b_eval_default_corpus.json", run)

	service := NewBenchmarkSurfaceService(BenchmarkSurfaceOptions{SampleDir: dir})

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
	if list[0].CostSummary.Precision != BenchmarkCostPrecisionEstimatedFromTokens {
		t.Fatalf("expected estimated cost summary, got %+v", list[0].CostSummary)
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
	if !strings.Contains(markdown, "Cost-ish USD") {
		t.Fatalf("expected markdown export to include cost-ish summary, got %s", markdown)
	}
}

func TestBenchmarkSurfaceServiceCompare(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	left := sampleEvalRun("phase6a-default", "phase6a-run", 11, 11, 0, 640)
	right := sampleEvalRun("phase6b-default", "phase6b-run", 4, 4, 0, 780)
	writeEvalRunSample(t, dir, "phase6a_eval_default_corpus.json", left)
	writeEvalRunSample(t, dir, "phase6b_eval_default_corpus.json", right)

	service := NewBenchmarkSurfaceService(BenchmarkSurfaceOptions{SampleDir: dir})
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

	service := NewBenchmarkSurfaceService(BenchmarkSurfaceOptions{SampleDir: filepath.Join(t.TempDir(), "missing")})
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

func writeEvalRunArtifact(t *testing.T, store runtime.ArtifactMetadataStore, workflowID string, artifactID string, run eval.EvalRun, costJSON string) {
	t.Helper()
	payload, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		t.Fatalf("marshal eval run artifact: %v", err)
	}
	content := string(payload)
	if strings.TrimSpace(costJSON) != "" {
		var raw map[string]any
		if err := json.Unmarshal(payload, &raw); err != nil {
			t.Fatalf("decode eval run payload: %v", err)
		}
		var cost any
		if err := json.Unmarshal([]byte(costJSON), &cost); err != nil {
			t.Fatalf("decode cost summary: %v", err)
		}
		raw["cost_summary"] = cost
		enriched, err := json.MarshalIndent(raw, "", "  ")
		if err != nil {
			t.Fatalf("encode enriched eval run artifact: %v", err)
		}
		content = string(enriched)
	}
	if err := store.SaveArtifact(workflowID, "task-benchmark-artifact", reporting.WorkflowArtifact{
		ID:          artifactID,
		WorkflowID:  workflowID,
		TaskID:      "task-benchmark-artifact",
		Kind:        reporting.ArtifactKindEvalRunResult,
		ProducedBy:  "benchmark-test",
		ContentJSON: content,
		Ref: reporting.ArtifactRef{
			ID:       artifactID,
			Location: artifactID + ".json",
			Summary:  "eval run artifact",
		},
		CreatedAt: run.CompletedAt,
	}); err != nil {
		t.Fatalf("save eval run artifact: %v", err)
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
