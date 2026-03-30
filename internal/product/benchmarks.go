package product

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/kobelakers/personal-cfo-os/internal/eval"
	"github.com/kobelakers/personal-cfo-os/internal/runtime"
)

type BenchmarkSurfaceService struct {
	sampleDir string
}

func NewBenchmarkSurfaceService(sampleDir string) *BenchmarkSurfaceService {
	return &BenchmarkSurfaceService{sampleDir: strings.TrimSpace(sampleDir)}
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
		fmt.Sprintf("- Corpus: `%s`", detail.Summary.CorpusID),
		fmt.Sprintf("- Deterministic: `%t`", detail.Summary.DeterministicOnly),
		fmt.Sprintf("- Passed: `%d`", detail.Summary.PassedCount),
		fmt.Sprintf("- Failed: `%d`", detail.Summary.FailedCount),
		fmt.Sprintf("- Average latency (ms): `%.2f`", detail.Summary.AverageLatencyMs),
		fmt.Sprintf("- Total token usage: `%d`", detail.Summary.TotalTokenUsage),
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

type benchmarkRunFile struct {
	Summary BenchmarkRunSummary
	Run     eval.EvalRun
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

func (s *BenchmarkSurfaceService) loadRuns() ([]benchmarkRunFile, error) {
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
	result := make([]benchmarkRunFile, 0)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(s.sampleDir, entry.Name())
		payload, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var run eval.EvalRun
		if err := json.Unmarshal(payload, &run); err != nil {
			continue
		}
		if strings.TrimSpace(run.RunID) == "" || len(run.Results) == 0 {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		result = append(result, benchmarkRunFile{
			Summary: NewBenchmarkRunSummary(id, "docs/eval/samples", run),
			Run:     run,
		})
	}
	return result, nil
}
