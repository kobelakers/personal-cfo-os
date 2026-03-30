package product

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/kobelakers/personal-cfo-os/internal/eval"
	"github.com/kobelakers/personal-cfo-os/internal/reporting"
	"github.com/kobelakers/personal-cfo-os/internal/runtime"
)

type BenchmarkSurfaceOptions struct {
	SampleDir    string
	Artifacts    runtime.ArtifactMetadataStore
	WorkflowRuns runtime.WorkflowRunStore
}

type BenchmarkSurfaceService struct {
	sampleDir    string
	artifacts    runtime.ArtifactMetadataStore
	workflowRuns runtime.WorkflowRunStore
}

type benchmarkRunEntry struct {
	Summary        BenchmarkRunSummary
	Run            eval.EvalRun
	DedupeKey      string
	PreferenceRank int
}

func NewBenchmarkSurfaceService(options BenchmarkSurfaceOptions) *BenchmarkSurfaceService {
	return &BenchmarkSurfaceService{
		sampleDir:    strings.TrimSpace(options.SampleDir),
		artifacts:    options.Artifacts,
		workflowRuns: options.WorkflowRuns,
	}
}

func (s *BenchmarkSurfaceService) ListRuns() ([]BenchmarkRunSummary, error) {
	entries, err := s.loadRuns()
	if err != nil {
		return nil, err
	}
	result := make([]BenchmarkRunSummary, 0, len(entries))
	for _, item := range entries {
		result = append(result, item.Summary)
	}
	slices.SortFunc(result, func(a, b BenchmarkRunSummary) int {
		switch {
		case a.StartedAt.Before(b.StartedAt):
			return 1
		case a.StartedAt.After(b.StartedAt):
			return -1
		case a.ID < b.ID:
			return -1
		case a.ID > b.ID:
			return 1
		default:
			return 0
		}
	})
	return result, nil
}

func (s *BenchmarkSurfaceService) GetRun(id string) (BenchmarkRunDetail, error) {
	item, ok, err := s.loadRun(strings.TrimSpace(id))
	if err != nil {
		return BenchmarkRunDetail{}, err
	}
	if !ok {
		return BenchmarkRunDetail{}, &runtime.NotFoundError{Resource: "benchmark_run", ID: strings.TrimSpace(id)}
	}
	return item, nil
}

func (s *BenchmarkSurfaceService) Compare(leftID string, rightID string) (BenchmarkCompareView, error) {
	left, err := s.GetRun(leftID)
	if err != nil {
		return BenchmarkCompareView{}, err
	}
	right, err := s.GetRun(rightID)
	if err != nil {
		return BenchmarkCompareView{}, err
	}
	diff := eval.CompareRuns(left.Run, right.Run)
	return BenchmarkCompareView{
		Left:        left.Summary,
		Right:       right.Summary,
		Diff:        diff,
		Summary:     append([]string{}, diff.Summary...),
		Description: fmt.Sprintf("%s vs %s", left.Summary.ID, right.Summary.ID),
	}, nil
}

func (s *BenchmarkSurfaceService) ExportRunMarkdown(id string) (string, error) {
	detail, err := s.GetRun(id)
	if err != nil {
		return "", err
	}
	lines := []string{
		fmt.Sprintf("# Benchmark %s", detail.Summary.Title),
		"",
		fmt.Sprintf("- ID: `%s`", detail.Summary.ID),
		fmt.Sprintf("- Source: `%s`", detail.Summary.Source),
		fmt.Sprintf("- Source Ref: `%s`", firstNonEmpty(detail.Summary.SourceRef, "n/a")),
		fmt.Sprintf("- Artifact ID: `%s`", firstNonEmpty(detail.Summary.ArtifactID, "n/a")),
		fmt.Sprintf("- Run ID: `%s`", detail.Summary.RunID),
		fmt.Sprintf("- Corpus: `%s`", detail.Summary.CorpusID),
		fmt.Sprintf("- Deterministic: `%t`", detail.Summary.DeterministicOnly),
		fmt.Sprintf("- Passed: `%d`", detail.Summary.PassedCount),
		fmt.Sprintf("- Failed: `%d`", detail.Summary.FailedCount),
		fmt.Sprintf("- Average latency (ms): `%.2f`", detail.Summary.AverageLatencyMs),
		fmt.Sprintf("- Total token usage: `%d`", detail.Summary.TotalTokenUsage),
		fmt.Sprintf("- Cost-ish USD: `%.6f` (%s via %s)", detail.Summary.CostSummary.USD, detail.Summary.CostSummary.Precision, firstNonEmpty(detail.Summary.CostSummary.Source, "unknown")),
		fmt.Sprintf("- Approval frequency: `%.2f`", detail.Summary.ApprovalFrequency),
		fmt.Sprintf("- Validator pass rate: `%.2f`", detail.Summary.ValidatorPassRate),
		"",
		"## Summary",
	}
	lines = append(lines, detail.Run.Summary.SummaryLines...)
	if len(detail.Run.Summary.PassedScenarios) > 0 {
		lines = append(lines, "", "## Passed Scenarios")
		for _, item := range detail.Run.Summary.PassedScenarios {
			lines = append(lines, fmt.Sprintf("- `%s`", item))
		}
	}
	if len(detail.Run.Summary.FailedScenarios) > 0 {
		lines = append(lines, "", "## Failed Scenarios")
		for _, item := range detail.Run.Summary.FailedScenarios {
			lines = append(lines, fmt.Sprintf("- `%s`", item))
		}
	}
	return strings.Join(lines, "\n"), nil
}

func (s *BenchmarkSurfaceService) loadRun(id string) (BenchmarkRunDetail, bool, error) {
	items, err := s.loadRuns()
	if err != nil {
		return BenchmarkRunDetail{}, false, err
	}
	for _, item := range items {
		if item.Summary.ID == id {
			return BenchmarkRunDetail{Summary: item.Summary, Run: item.Run}, true, nil
		}
	}
	return BenchmarkRunDetail{}, false, nil
}

func (s *BenchmarkSurfaceService) loadRuns() ([]benchmarkRunEntry, error) {
	sampleEntries, err := s.loadSampleRuns()
	if err != nil {
		return nil, err
	}
	artifactEntries, err := s.loadArtifactRuns()
	if err != nil {
		return nil, err
	}

	merged := make(map[string]benchmarkRunEntry)
	for _, item := range append(sampleEntries, artifactEntries...) {
		key := item.DedupeKey
		if key == "" {
			key = item.Summary.ID
		}
		existing, ok := merged[key]
		if !ok || preferBenchmarkEntry(item, existing) {
			merged[key] = item
		}
	}

	result := make([]benchmarkRunEntry, 0, len(merged))
	for _, item := range merged {
		result = append(result, item)
	}
	slices.SortFunc(result, func(a, b benchmarkRunEntry) int {
		switch {
		case a.Summary.StartedAt.Before(b.Summary.StartedAt):
			return 1
		case a.Summary.StartedAt.After(b.Summary.StartedAt):
			return -1
		case a.Summary.ID < b.Summary.ID:
			return -1
		case a.Summary.ID > b.Summary.ID:
			return 1
		default:
			return 0
		}
	})
	return result, nil
}

func (s *BenchmarkSurfaceService) loadSampleRuns() ([]benchmarkRunEntry, error) {
	if s == nil || s.sampleDir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(s.sampleDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	result := make([]benchmarkRunEntry, 0)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(s.sampleDir, entry.Name())
		payload, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		run, ok := decodeBenchmarkRun(payload)
		if !ok {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		cost := benchmarkCostSummaryFromPayload(payload, run)
		result = append(result, benchmarkRunEntry{
			Summary:        NewBenchmarkRunSummary(id, "sample", path, "", run, cost),
			Run:            run,
			DedupeKey:      benchmarkRunDedupeKey(run, ""),
			PreferenceRank: 0,
		})
	}
	return result, nil
}

func (s *BenchmarkSurfaceService) loadArtifactRuns() ([]benchmarkRunEntry, error) {
	if s == nil || s.artifacts == nil || s.workflowRuns == nil {
		return nil, nil
	}
	workflows, err := s.workflowRuns.List()
	if err != nil {
		return nil, err
	}
	result := make([]benchmarkRunEntry, 0)
	for _, workflow := range workflows {
		artifacts, err := s.artifacts.ListArtifactsByWorkflow(workflow.WorkflowID)
		if err != nil {
			return nil, err
		}
		for _, item := range artifacts {
			if item.Kind != reporting.ArtifactKindEvalRunResult {
				continue
			}
			artifact, ok, err := s.artifacts.LoadArtifact(item.ID)
			if err != nil {
				return nil, err
			}
			if !ok || strings.TrimSpace(artifact.ContentJSON) == "" {
				continue
			}
			payload := []byte(artifact.ContentJSON)
			run, ok := decodeBenchmarkRun(payload)
			if !ok {
				continue
			}
			result = append(result, benchmarkRunEntry{
				Summary: NewBenchmarkRunSummary(
					benchmarkArtifactID(artifact.ID),
					"artifact",
					firstNonEmpty(artifact.Ref.Location, artifact.Ref.ID, artifact.ID),
					artifact.ID,
					run,
					benchmarkCostSummaryFromPayload(payload, run),
				),
				Run:            run,
				DedupeKey:      benchmarkRunDedupeKey(run, artifact.ID),
				PreferenceRank: 1,
			})
		}
	}
	return result, nil
}

func decodeBenchmarkRun(payload []byte) (eval.EvalRun, bool) {
	var run eval.EvalRun
	if err := json.Unmarshal(payload, &run); err != nil {
		return eval.EvalRun{}, false
	}
	if strings.TrimSpace(run.RunID) == "" || len(run.Results) == 0 {
		return eval.EvalRun{}, false
	}
	return run, true
}

func benchmarkRunDedupeKey(run eval.EvalRun, fallback string) string {
	return firstNonEmpty(strings.TrimSpace(run.RunID), strings.TrimSpace(fallback))
}

func benchmarkArtifactID(artifactID string) string {
	return "artifact-" + strings.TrimSpace(artifactID)
}

func preferBenchmarkEntry(candidate benchmarkRunEntry, existing benchmarkRunEntry) bool {
	if candidate.PreferenceRank != existing.PreferenceRank {
		return candidate.PreferenceRank < existing.PreferenceRank
	}
	switch {
	case candidate.Summary.CompletedAt.After(existing.Summary.CompletedAt):
		return true
	case candidate.Summary.CompletedAt.Before(existing.Summary.CompletedAt):
		return false
	default:
		return candidate.Summary.ID < existing.Summary.ID
	}
}
