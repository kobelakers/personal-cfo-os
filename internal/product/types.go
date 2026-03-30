package product

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/eval"
	"github.com/kobelakers/personal-cfo-os/internal/model"
	"github.com/kobelakers/personal-cfo-os/internal/observability"
	"github.com/kobelakers/personal-cfo-os/internal/reporting"
	"github.com/kobelakers/personal-cfo-os/internal/runtime"
)

type ProfileMeta struct {
	RuntimeProfile          string   `json:"runtime_profile"`
	RuntimeBackend          string   `json:"runtime_backend"`
	BlobBackend             string   `json:"blob_backend"`
	UIMode                  string   `json:"ui_mode"`
	SupportedSchemaVersions []string `json:"supported_schema_versions"`
	BenchmarkCatalog        []string `json:"benchmark_catalog,omitempty"`
}

type ApprovalDetail struct {
	Approval   runtime.ApprovalStateRecord `json:"approval"`
	TaskGraph  *runtime.TaskGraphView      `json:"task_graph,omitempty"`
	ReplayHint observability.ReplayQuery   `json:"replay_hint"`
}

type ArtifactUsageSummary struct {
	Providers        []string  `json:"providers,omitempty"`
	Models           []string  `json:"models,omitempty"`
	PromptIDs        []string  `json:"prompt_ids,omitempty"`
	TotalTokens      int       `json:"total_tokens,omitempty"`
	EstimatedCostUSD float64   `json:"estimated_cost_usd,omitempty"`
	CallCount        int       `json:"call_count,omitempty"`
	LastRecordedAt   time.Time `json:"last_recorded_at,omitempty"`
}

type ArtifactContentView struct {
	Artifact      reporting.WorkflowArtifact `json:"artifact"`
	ContentType   string                     `json:"content_type"`
	Structured    any                        `json:"structured,omitempty"`
	RawText       string                     `json:"raw_text,omitempty"`
	UsageSummary  ArtifactUsageSummary       `json:"usage_summary,omitempty"`
	SummaryLines  []string                   `json:"summary_lines,omitempty"`
	ReferenceOnly bool                       `json:"reference_only,omitempty"`
}

type BenchmarkRunSummary struct {
	ID                       string    `json:"id"`
	Source                   string    `json:"source"`
	Title                    string    `json:"title"`
	CorpusID                 string    `json:"corpus_id"`
	RunID                    string    `json:"run_id"`
	DeterministicOnly        bool      `json:"deterministic_only"`
	ScenarioCount            int       `json:"scenario_count"`
	PassedCount              int       `json:"passed_count"`
	FailedCount              int       `json:"failed_count"`
	ApprovalFrequency        float64   `json:"approval_frequency"`
	AverageLatencyMs         float64   `json:"average_latency_ms"`
	TotalTokenUsage          int       `json:"total_token_usage"`
	ValidatorPassRate        float64   `json:"validator_pass_rate"`
	PolicyViolationRate      float64   `json:"policy_violation_rate"`
	StartedAt                time.Time `json:"started_at"`
	CompletedAt              time.Time `json:"completed_at"`
}

type BenchmarkRunDetail struct {
	Summary BenchmarkRunSummary `json:"summary"`
	Run     eval.EvalRun        `json:"run"`
}

type BenchmarkCompareView struct {
	Left        BenchmarkRunSummary `json:"left"`
	Right       BenchmarkRunSummary `json:"right"`
	Diff        eval.EvalDiff       `json:"diff"`
	Summary     []string            `json:"summary,omitempty"`
	Description string              `json:"description,omitempty"`
}

func BuildArtifactContentView(artifact reporting.WorkflowArtifact) ArtifactContentView {
	view := ArtifactContentView{
		Artifact:    artifact,
		ContentType: "application/json",
	}
	payload := strings.TrimSpace(artifact.ContentJSON)
	if payload == "" {
		view.ReferenceOnly = true
		view.SummaryLines = []string{
			fmt.Sprintf("artifact=%s", artifact.ID),
			fmt.Sprintf("kind=%s", artifact.Kind),
			fmt.Sprintf("location=%s", artifact.Ref.Location),
		}
		return view
	}

	var structured any
	if err := json.Unmarshal([]byte(payload), &structured); err != nil {
		view.ContentType = "text/plain"
		view.RawText = payload
		view.SummaryLines = []string{artifactSummaryLine(artifact)}
		return view
	}
	view.Structured = structured
	view.SummaryLines = []string{artifactSummaryLine(artifact)}

	switch artifact.Kind {
	case reporting.ArtifactKindReplayBundle:
		var bundle observability.ReplayBundle
		if err := json.Unmarshal([]byte(payload), &bundle); err == nil {
			view.UsageSummary = buildUsageSummary(bundle.Trace.Usage, bundle.Trace.LLMCalls)
			view.SummaryLines = append(view.SummaryLines,
				fmt.Sprintf("scenario=%s", bundle.Scenario),
				fmt.Sprintf("trace_id=%s", bundle.Trace.TraceID),
				fmt.Sprintf("llm_calls=%d", len(bundle.Trace.LLMCalls)),
				fmt.Sprintf("usage_records=%d", len(bundle.Trace.Usage)),
			)
		}
	case reporting.ArtifactKindReplaySummary:
		var summary observability.DebugSummary
		if err := json.Unmarshal([]byte(payload), &summary); err == nil {
			view.SummaryLines = append(view.SummaryLines,
				fmt.Sprintf("workflow_id=%s", summary.WorkflowID),
				fmt.Sprintf("final_runtime_state=%s", summary.FinalRuntimeState),
			)
		}
	case reporting.ArtifactKindEvalRunResult:
		var run eval.EvalRun
		if err := json.Unmarshal([]byte(payload), &run); err == nil {
			view.SummaryLines = append(view.SummaryLines,
				fmt.Sprintf("corpus_id=%s", run.CorpusID),
				fmt.Sprintf("passed=%d failed=%d", run.Score.PassedCount, run.Score.FailedCount),
			)
			view.UsageSummary = ArtifactUsageSummary{
				TotalTokens:      run.Score.TotalTokenUsage,
				EstimatedCostUSD: estimatedCostFromEvalRun(run),
			}
		}
	}

	return view
}

func NewBenchmarkRunSummary(id string, source string, run eval.EvalRun) BenchmarkRunSummary {
	return BenchmarkRunSummary{
		ID:                  id,
		Source:              source,
		Title:               firstNonEmpty(run.CorpusID, id),
		CorpusID:            run.CorpusID,
		RunID:               run.RunID,
		DeterministicOnly:   run.DeterministicOnly,
		ScenarioCount:       run.Score.ScenarioCount,
		PassedCount:         run.Score.PassedCount,
		FailedCount:         run.Score.FailedCount,
		ApprovalFrequency:   run.Score.ApprovalFrequency,
		AverageLatencyMs:    run.Score.AverageLatencyMilliseconds,
		TotalTokenUsage:     run.Score.TotalTokenUsage,
		ValidatorPassRate:   run.Score.ValidatorPassRate,
		PolicyViolationRate: run.Score.PolicyViolationRate,
		StartedAt:           run.StartedAt,
		CompletedAt:         run.CompletedAt,
	}
}

func buildUsageSummary(usage []model.UsageRecord, calls []model.CallRecord) ArtifactUsageSummary {
	summary := ArtifactUsageSummary{}
	providers := make(map[string]struct{})
	models := make(map[string]struct{})
	promptIDs := make(map[string]struct{})
	for _, item := range usage {
		summary.TotalTokens += item.TotalTokens
		summary.EstimatedCostUSD += item.EstimatedCostUSD
		if item.RecordedAt.After(summary.LastRecordedAt) {
			summary.LastRecordedAt = item.RecordedAt
		}
		if strings.TrimSpace(item.Provider) != "" {
			providers[item.Provider] = struct{}{}
		}
		if strings.TrimSpace(item.Model) != "" {
			models[item.Model] = struct{}{}
		}
		if strings.TrimSpace(item.PromptID) != "" {
			promptIDs[item.PromptID] = struct{}{}
		}
	}
	summary.CallCount = len(calls)
	for provider := range providers {
		summary.Providers = append(summary.Providers, provider)
	}
	for modelName := range models {
		summary.Models = append(summary.Models, modelName)
	}
	for promptID := range promptIDs {
		summary.PromptIDs = append(summary.PromptIDs, promptID)
	}
	return summary
}

func estimatedCostFromEvalRun(run eval.EvalRun) float64 {
	return 0
}

func artifactSummaryLine(artifact reporting.WorkflowArtifact) string {
	return fmt.Sprintf("artifact=%s kind=%s", artifact.ID, artifact.Kind)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
