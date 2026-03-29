package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/app"
	"github.com/kobelakers/personal-cfo-os/internal/memory"
	"github.com/kobelakers/personal-cfo-os/internal/model"
	"github.com/kobelakers/personal-cfo-os/internal/state"
)

func main() {
	var (
		phase           = flag.String("phase", "5b", "phase runner: 5b or 5c")
		providerMode    = flag.String("provider-mode", "mock", "provider mode: mock or live")
		fixtureDir      = flag.String("fixture-dir", filepath.Join("tests", "fixtures"), "fixture directory")
		holdingsFixture = flag.String("holdings-fixture", "holdings_2026-03-safe.csv", "holdings fixture file")
		memoryDB        = flag.String("memory-db", os.Getenv("MEMORY_DB_PATH"), "memory sqlite db path for phase 5c")
		userID          = flag.String("user-id", "user-1", "user id")
		rawInput        = flag.String("input", "请帮我做一份月度财务复盘", "monthly review input")
		traceOut        = flag.String("trace-out", "", "trace output path")
		artifactOut     = flag.String("artifact-out", "", "artifact output path")
		reindexMemory   = flag.Bool("reindex-memory", false, "rebuild memory embeddings and lexical indexes before running (phase 5c only)")
		indexOnly       = flag.Bool("index-only", false, "rebuild memory indexes and exit without running workflow (phase 5c only)")
		fixedNow        = flag.String("fixed-now", "2026-03-29T08:00:00Z", "fixed UTC time for reproducible runs")
	)
	flag.Parse()

	now, err := time.Parse(time.RFC3339, *fixedNow)
	if err != nil {
		failf("parse fixed-now: %v", err)
	}
	switch *phase {
	case "5b":
		runPhase5B(*providerMode, *fixtureDir, *holdingsFixture, now, *userID, *rawInput, *traceOut, *artifactOut)
	case "5c":
		runPhase5C(*providerMode, *fixtureDir, *holdingsFixture, *memoryDB, *reindexMemory, *indexOnly, now, *userID, *rawInput, *traceOut, *artifactOut)
	default:
		failf("unsupported phase %q", *phase)
	}
}

func failf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func runPhase5B(providerMode string, fixtureDir string, holdingsFixture string, now time.Time, userID string, rawInput string, traceOut string, artifactOut string) {
	options := app.MonthlyReview5BOptions{
		FixtureDir:      fixtureDir,
		HoldingsFixture: holdingsFixture,
		Now:             func() time.Time { return now.UTC() },
	}
	switch providerMode {
	case "mock":
		options.ChatModelFactory = func(callRecorder model.CallRecorder, usageRecorder model.UsageRecorder) model.ChatModel {
			return app.NewMockMonthlyReviewChatModelWithTrace(callRecorder, usageRecorder)
		}
	case "live":
		options.ChatModelFactory = func(callRecorder model.CallRecorder, usageRecorder model.UsageRecorder) model.ChatModel {
			return app.NewLiveMonthlyReviewChatModel(callRecorder, usageRecorder)
		}
	default:
		failf("unsupported provider-mode %q", providerMode)
	}
	env, err := app.OpenMonthlyReview5BEnvironment(options)
	if err != nil {
		failf("open monthly review 5b environment: %v", err)
	}
	result, err := env.Run(context.Background(), userID, rawInput, state.FinancialWorldState{})
	if err != nil {
		failf("run monthly review 5b: %v", err)
	}
	writeOutputs(result.Result.WorkflowID, string(result.Result.RuntimeState), providerMode, result.Result.Report.Summary, len(result.Trace.LLMCalls), len(result.Trace.PromptRenders), len(result.Trace.StructuredOutputs), traceOut, artifactOut, result.WriteTrace, result.WriteArtifact)
}

func runPhase5C(providerMode string, fixtureDir string, holdingsFixture string, memoryDB string, reindexMemory bool, indexOnly bool, now time.Time, userID string, rawInput string, traceOut string, artifactOut string) {
	if memoryDB == "" {
		failf("--memory-db or MEMORY_DB_PATH is required for phase 5c")
	}
	options := app.MonthlyReview5COptions{
		FixtureDir:      fixtureDir,
		HoldingsFixture: holdingsFixture,
		MemoryDBPath:    memoryDB,
		Now:             func() time.Time { return now.UTC() },
	}
	switch providerMode {
	case "mock":
		options.ChatModelFactory = func(callRecorder model.CallRecorder, usageRecorder model.UsageRecorder) model.ChatModel {
			return app.NewMockMonthlyReviewChatModelWithTrace(callRecorder, usageRecorder)
		}
		options.EmbeddingProviderFactory = func(callRecorder memory.EmbeddingCallRecorder, usageRecorder memory.EmbeddingUsageRecorder) memory.EmbeddingProvider {
			return app.NewMockMonthlyReviewEmbeddingProvider(callRecorder, usageRecorder)
		}
		options.EmbeddingModel = "mock-embedding-model"
	case "live":
		options.ChatModelFactory = func(callRecorder model.CallRecorder, usageRecorder model.UsageRecorder) model.ChatModel {
			return app.NewLiveMonthlyReviewChatModel(callRecorder, usageRecorder)
		}
		options.EmbeddingProviderFactory = func(callRecorder memory.EmbeddingCallRecorder, usageRecorder memory.EmbeddingUsageRecorder) memory.EmbeddingProvider {
			return app.NewLiveMonthlyReviewEmbeddingProvider(callRecorder, usageRecorder)
		}
		options.EmbeddingModel = strings.TrimSpace(os.Getenv("OPENAI_EMBEDDING_MODEL"))
	default:
		failf("unsupported provider-mode %q", providerMode)
	}
	env, err := app.OpenMonthlyReview5CEnvironment(options)
	if err != nil {
		failf("open monthly review 5c environment: %v", err)
	}
	defer func() { _ = env.Close() }()
	if reindexMemory {
		summary, err := env.RebuildMemoryIndexes(context.Background())
		if err != nil {
			failf("rebuild memory indexes: %v", err)
		}
		fmt.Printf("memory_index_records=%d embeddings=%d terms=%d model=%s\n", summary.RecordsIndexed, summary.EmbeddingsBuilt, summary.TermsBuilt, summary.Model)
	}
	if indexOnly {
		return
	}
	result, err := env.Run(context.Background(), userID, rawInput, state.FinancialWorldState{})
	if err != nil {
		failf("run monthly review 5c: %v", err)
	}
	writeOutputs(result.Result.WorkflowID, string(result.Result.RuntimeState), providerMode, result.Result.Report.Summary, len(result.Trace.LLMCalls), len(result.Trace.PromptRenders), len(result.Trace.StructuredOutputs), traceOut, artifactOut, result.WriteTrace, result.WriteArtifact)
}

func writeOutputs(workflowID string, runtimeState string, providerMode string, reportSummary string, llmCalls int, promptRenders int, structuredOutputs int, traceOut string, artifactOut string, writeTrace func(string) error, writeArtifact func(string) error) {
	if artifactOut != "" {
		if err := writeArtifact(artifactOut); err != nil {
			failf("write artifact: %v", err)
		}
	}
	if traceOut != "" {
		if err := writeTrace(traceOut); err != nil {
			failf("write trace: %v", err)
		}
	}
	fmt.Printf("workflow_id=%s\nruntime_state=%s\nprovider_mode=%s\nreport_summary=%s\n", workflowID, runtimeState, providerMode, reportSummary)
	fmt.Printf("trace_llm_calls=%d prompt_renders=%d structured_outputs=%d\n", llmCalls, promptRenders, structuredOutputs)
}
