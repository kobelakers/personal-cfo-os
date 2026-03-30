package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/app"
	inteval "github.com/kobelakers/personal-cfo-os/internal/eval"
	"github.com/kobelakers/personal-cfo-os/internal/memory"
	"github.com/kobelakers/personal-cfo-os/internal/model"
	"github.com/kobelakers/personal-cfo-os/internal/state"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer, stderr io.Writer) error {
	fs := flag.NewFlagSet("eval", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var (
		mode            = fs.String("mode", "phase", "eval mode: phase or corpus")
		phase           = fs.String("phase", "5b", "phase runner: 5b, 5c, or 5d")
		workflowName    = fs.String("workflow", "monthly_review", "workflow runner for phase 5d: monthly_review or debt_vs_invest")
		providerMode    = fs.String("provider-mode", "mock", "provider mode: mock or live")
		fixtureDir      = fs.String("fixture-dir", filepath.Join("tests", "fixtures"), "fixture directory")
		holdingsFixture = fs.String("holdings-fixture", "holdings_2026-03-safe.csv", "holdings fixture file")
		memoryDB        = fs.String("memory-db", os.Getenv("MEMORY_DB_PATH"), "memory sqlite db path for phase 5c/5d")
		runtimeDB       = fs.String("runtime-db", os.Getenv("RUNTIME_DB_PATH"), "runtime sqlite db path for phase 5d")
		userID          = fs.String("user-id", "user-1", "user id")
		rawInput        = fs.String("input", "", "workflow input; defaults to a workflow-specific prompt when omitted")
		traceOut        = fs.String("trace-out", "", "trace output path")
		artifactOut     = fs.String("artifact-out", "", "artifact output path")
		reindexMemory   = fs.Bool("reindex-memory", false, "rebuild memory embeddings and lexical indexes before running (phase 5c/5d)")
		indexOnly       = fs.Bool("index-only", false, "rebuild memory indexes and exit without running workflow (phase 5c/5d)")
		fixedNow        = fs.String("fixed-now", "2026-03-29T08:00:00Z", "fixed UTC time for reproducible runs")
		corpusID        = fs.String("corpus", "phase6a-default", "eval corpus id for --mode corpus")
		scenarioID      = fs.String("scenario", "", "single scenario id for --mode corpus")
		outputPath      = fs.String("output", "", "eval run output path")
		format          = fs.String("format", "summary", "output format for corpus/compare mode: json or summary")
		workDir         = fs.String("workdir", "", "optional persistent work directory for corpus runs")
		compareLeft     = fs.String("compare-left", "", "compare left eval result json file")
		compareRight    = fs.String("compare-right", "", "compare right eval result json file")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}

	now, err := time.Parse(time.RFC3339, *fixedNow)
	if err != nil {
		return fmt.Errorf("parse fixed-now: %w", err)
	}
	if strings.TrimSpace(*compareLeft) != "" || strings.TrimSpace(*compareRight) != "" {
		if strings.TrimSpace(*compareLeft) == "" || strings.TrimSpace(*compareRight) == "" {
			return fmt.Errorf("--compare-left and --compare-right must be provided together")
		}
		return compareEvalRuns(*compareLeft, *compareRight, *format, stdout)
	}

	switch strings.TrimSpace(*mode) {
	case "phase":
		switch *phase {
		case "5b":
			return runPhase5B(stdout, *providerMode, *fixtureDir, *holdingsFixture, now, *userID, resolvedInput(*rawInput, "monthly_review"), *traceOut, *artifactOut)
		case "5c":
			return runPhase5C(stdout, *providerMode, *fixtureDir, *holdingsFixture, *memoryDB, *reindexMemory, *indexOnly, now, *userID, resolvedInput(*rawInput, "monthly_review"), *traceOut, *artifactOut)
		case "5d":
			return runPhase5D(stdout, *providerMode, *workflowName, *fixtureDir, *holdingsFixture, *memoryDB, *runtimeDB, *reindexMemory, *indexOnly, now, *userID, resolvedInput(*rawInput, *workflowName), *traceOut, *artifactOut)
		default:
			return fmt.Errorf("unsupported phase %q", *phase)
		}
	case "corpus":
		corpus, err := loadCorpus(*corpusID)
		if err != nil {
			return err
		}
		if strings.TrimSpace(*scenarioID) != "" {
			item, ok := selectScenario(corpus, strings.TrimSpace(*scenarioID))
			if !ok {
				return fmt.Errorf("scenario %q not found in corpus %q", *scenarioID, corpus.ID)
			}
			corpus.Cases = []inteval.ScenarioCase{item}
		}
		harness := inteval.NewHarness(inteval.HarnessOptions{
			FixtureDir: *fixtureDir,
			WorkDir:    *workDir,
			Now:        func() time.Time { return now.UTC() },
		})
		runResult, err := harness.RunCorpus(context.Background(), corpus)
		if err != nil {
			return err
		}
		if *outputPath != "" {
			if err := writeJSONFile(*outputPath, runResult); err != nil {
				return err
			}
		}
		printEvalRun(stdout, runResult, *format)
		return nil
	default:
		return fmt.Errorf("unsupported mode %q", *mode)
	}
}

func runPhase5B(stdout io.Writer, providerMode string, fixtureDir string, holdingsFixture string, now time.Time, userID string, rawInput string, traceOut string, artifactOut string) error {
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
		return fmt.Errorf("unsupported provider-mode %q", providerMode)
	}
	env, err := app.OpenMonthlyReview5BEnvironment(options)
	if err != nil {
		return fmt.Errorf("open monthly review 5b environment: %w", err)
	}
	result, err := env.Run(context.Background(), userID, rawInput, state.FinancialWorldState{})
	if err != nil {
		return fmt.Errorf("run monthly review 5b: %w", err)
	}
	return writeOutputs(stdout, result.Result.WorkflowID, string(result.Result.RuntimeState), providerMode, result.Result.Report.Summary, len(result.Trace.LLMCalls), len(result.Trace.PromptRenders), len(result.Trace.StructuredOutputs), traceOut, artifactOut, result.WriteTrace, result.WriteArtifact)
}

func runPhase5C(stdout io.Writer, providerMode string, fixtureDir string, holdingsFixture string, memoryDB string, reindexMemory bool, indexOnly bool, now time.Time, userID string, rawInput string, traceOut string, artifactOut string) error {
	if memoryDB == "" {
		return fmt.Errorf("--memory-db or MEMORY_DB_PATH is required for phase 5c")
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
		return fmt.Errorf("unsupported provider-mode %q", providerMode)
	}
	env, err := app.OpenMonthlyReview5CEnvironment(options)
	if err != nil {
		return fmt.Errorf("open monthly review 5c environment: %w", err)
	}
	defer func() { _ = env.Close() }()
	if reindexMemory {
		summary, err := env.RebuildMemoryIndexes(context.Background())
		if err != nil {
			return fmt.Errorf("rebuild memory indexes: %w", err)
		}
		fmt.Fprintf(stdout, "memory_index_records=%d embeddings=%d terms=%d model=%s\n", summary.RecordsIndexed, summary.EmbeddingsBuilt, summary.TermsBuilt, summary.Model)
	}
	if indexOnly {
		return nil
	}
	result, err := env.Run(context.Background(), userID, rawInput, state.FinancialWorldState{})
	if err != nil {
		return fmt.Errorf("run monthly review 5c: %w", err)
	}
	return writeOutputs(stdout, result.Result.WorkflowID, string(result.Result.RuntimeState), providerMode, result.Result.Report.Summary, len(result.Trace.LLMCalls), len(result.Trace.PromptRenders), len(result.Trace.StructuredOutputs), traceOut, artifactOut, result.WriteTrace, result.WriteArtifact)
}

func runPhase5D(stdout io.Writer, providerMode string, workflowName string, fixtureDir string, holdingsFixture string, memoryDB string, runtimeDB string, reindexMemory bool, indexOnly bool, now time.Time, userID string, rawInput string, traceOut string, artifactOut string) error {
	if memoryDB == "" {
		return fmt.Errorf("--memory-db or MEMORY_DB_PATH is required for phase 5d")
	}
	options := app.Phase5DOptions{
		FixtureDir:      fixtureDir,
		HoldingsFixture: holdingsFixture,
		MemoryDBPath:    memoryDB,
		RuntimeDBPath:   runtimeDB,
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
		return fmt.Errorf("unsupported provider-mode %q", providerMode)
	}

	env, err := app.OpenPhase5DEnvironment(options)
	if err != nil {
		return fmt.Errorf("open phase 5d environment: %w", err)
	}
	defer func() { _ = env.Close() }()
	if reindexMemory {
		summary, err := env.RebuildMemoryIndexes(context.Background())
		if err != nil {
			return fmt.Errorf("rebuild memory indexes: %w", err)
		}
		fmt.Fprintf(stdout, "memory_index_records=%d embeddings=%d terms=%d model=%s\n", summary.RecordsIndexed, summary.EmbeddingsBuilt, summary.TermsBuilt, summary.Model)
	}
	if indexOnly {
		return nil
	}

	switch workflowName {
	case "monthly_review":
		result, err := env.RunMonthlyReview(context.Background(), userID, rawInput, state.FinancialWorldState{})
		if err != nil {
			return fmt.Errorf("run monthly review 5d: %w", err)
		}
		return writeOutputs(stdout, result.Result.WorkflowID, string(result.Result.RuntimeState), providerMode, result.Result.Report.Summary, len(result.Trace.LLMCalls), len(result.Trace.PromptRenders), len(result.Trace.StructuredOutputs), traceOut, artifactOut, result.WriteTrace, result.WriteArtifact)
	case "debt_vs_invest":
		result, err := env.RunDebtVsInvest(context.Background(), userID, rawInput, state.FinancialWorldState{})
		if err != nil {
			return fmt.Errorf("run debt vs invest 5d: %w", err)
		}
		return writeOutputs(stdout, result.Result.WorkflowID, string(result.Result.RuntimeState), providerMode, result.Result.Report.Conclusion, len(result.Trace.LLMCalls), len(result.Trace.PromptRenders), len(result.Trace.StructuredOutputs), traceOut, artifactOut, result.WriteTrace, result.WriteArtifact)
	default:
		return fmt.Errorf("unsupported workflow %q for phase 5d", workflowName)
	}
}

func resolvedInput(rawInput string, workflowName string) string {
	if strings.TrimSpace(rawInput) != "" {
		return rawInput
	}
	switch workflowName {
	case "debt_vs_invest":
		return "提前还贷还是继续投资更合适"
	default:
		return "请帮我做一份月度财务复盘"
	}
}

func loadCorpus(id string) (inteval.ScenarioCorpus, error) {
	switch strings.TrimSpace(id) {
	case "", "default", "phase6a-default":
		return inteval.DefaultScenarioCorpus(), nil
	default:
		return inteval.ScenarioCorpus{}, fmt.Errorf("unsupported corpus %q", id)
	}
}

func selectScenario(corpus inteval.ScenarioCorpus, scenarioID string) (inteval.ScenarioCase, bool) {
	for _, item := range corpus.Cases {
		if item.ID == scenarioID {
			return item, true
		}
	}
	return inteval.ScenarioCase{}, false
}

func compareEvalRuns(leftPath string, rightPath string, format string, stdout io.Writer) error {
	left, err := readEvalRun(leftPath)
	if err != nil {
		return err
	}
	right, err := readEvalRun(rightPath)
	if err != nil {
		return err
	}
	diff := inteval.CompareRuns(left, right)
	if format == "json" {
		return encodeJSON(stdout, diff)
	}
	fmt.Fprintf(stdout, "compare_left=%s\ncompare_right=%s\n", left.RunID, right.RunID)
	for _, line := range diff.Summary {
		fmt.Fprintf(stdout, "- %s\n", line)
	}
	return nil
}

func printEvalRun(stdout io.Writer, run inteval.EvalRun, format string) {
	if format == "json" {
		if err := encodeJSON(stdout, run); err != nil {
			panic(err)
		}
		return
	}
	fmt.Fprintf(stdout, "run_id=%s\ncorpus=%s deterministic_only=%t\n", run.RunID, run.CorpusID, run.DeterministicOnly)
	fmt.Fprintf(stdout, "scenario_count=%d passed=%d failed=%d\n", run.Score.ScenarioCount, run.Score.PassedCount, run.Score.FailedCount)
	fmt.Fprintf(stdout, "task_success_rate=%.2f validator_pass_rate=%.2f approval_frequency=%.2f retry_frequency=%.2f\n", run.Score.TaskSuccessRate, run.Score.ValidatorPassRate, run.Score.ApprovalFrequency, run.Score.RetryFrequency)
	for _, item := range run.Results {
		fmt.Fprintf(stdout, "- %s passed=%t state=%s scope=%s:%s\n", item.ScenarioID, item.Passed, item.RuntimeState, item.Scope.Kind, item.Scope.ID)
	}
}

func writeOutputs(stdout io.Writer, workflowID string, runtimeState string, providerMode string, reportSummary string, llmCalls int, promptRenders int, structuredOutputs int, traceOut string, artifactOut string, writeTrace func(string) error, writeArtifact func(string) error) error {
	if artifactOut != "" {
		if err := writeArtifact(artifactOut); err != nil {
			return fmt.Errorf("write artifact: %w", err)
		}
	}
	if traceOut != "" {
		if err := writeTrace(traceOut); err != nil {
			return fmt.Errorf("write trace: %w", err)
		}
	}
	fmt.Fprintf(stdout, "workflow_id=%s\nruntime_state=%s\nprovider_mode=%s\nreport_summary=%s\n", workflowID, runtimeState, providerMode, reportSummary)
	fmt.Fprintf(stdout, "trace_llm_calls=%d prompt_renders=%d structured_outputs=%d\n", llmCalls, promptRenders, structuredOutputs)
	return nil
}

func readEvalRun(path string) (inteval.EvalRun, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return inteval.EvalRun{}, err
	}
	var runResult inteval.EvalRun
	if err := json.Unmarshal(payload, &runResult); err != nil {
		return inteval.EvalRun{}, err
	}
	return runResult, nil
}

func writeJSONFile(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, payload, 0o644)
}

func encodeJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
