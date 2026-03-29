package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/app"
	"github.com/kobelakers/personal-cfo-os/internal/model"
	"github.com/kobelakers/personal-cfo-os/internal/state"
)

func main() {
	var (
		providerMode    = flag.String("provider-mode", "mock", "provider mode: mock or live")
		fixtureDir      = flag.String("fixture-dir", filepath.Join("tests", "fixtures"), "fixture directory")
		holdingsFixture = flag.String("holdings-fixture", "holdings_2026-03-safe.csv", "holdings fixture file")
		userID          = flag.String("user-id", "user-1", "user id")
		rawInput        = flag.String("input", "请帮我做一份月度财务复盘", "monthly review input")
		traceOut        = flag.String("trace-out", "", "trace output path")
		artifactOut     = flag.String("artifact-out", "", "artifact output path")
		fixedNow        = flag.String("fixed-now", "2026-03-29T08:00:00Z", "fixed UTC time for reproducible runs")
	)
	flag.Parse()

	now, err := time.Parse(time.RFC3339, *fixedNow)
	if err != nil {
		failf("parse fixed-now: %v", err)
	}
	options := app.MonthlyReview5BOptions{
		FixtureDir:      *fixtureDir,
		HoldingsFixture: *holdingsFixture,
		Now:             func() time.Time { return now.UTC() },
	}
	switch *providerMode {
	case "mock":
		options.ChatModelFactory = func(callRecorder model.CallRecorder, usageRecorder model.UsageRecorder) model.ChatModel {
			return app.NewMockMonthlyReviewChatModelWithTrace(callRecorder, usageRecorder)
		}
	case "live":
		options.ChatModelFactory = func(callRecorder model.CallRecorder, usageRecorder model.UsageRecorder) model.ChatModel {
			return app.NewLiveMonthlyReviewChatModel(callRecorder, usageRecorder)
		}
	default:
		failf("unsupported provider-mode %q", *providerMode)
	}
	env, err := app.OpenMonthlyReview5BEnvironment(options)
	if err != nil {
		failf("open monthly review 5b environment: %v", err)
	}
	result, err := env.Run(context.Background(), *userID, *rawInput, state.FinancialWorldState{})
	if err != nil {
		failf("run monthly review 5b: %v", err)
	}
	if *artifactOut != "" {
		if err := result.WriteArtifact(*artifactOut); err != nil {
			failf("write artifact: %v", err)
		}
	}
	if *traceOut != "" {
		if err := result.WriteTrace(*traceOut); err != nil {
			failf("write trace: %v", err)
		}
	}
	fmt.Printf("workflow_id=%s\nruntime_state=%s\nprovider_mode=%s\nreport_summary=%s\n", result.Result.WorkflowID, result.Result.RuntimeState, *providerMode, result.Result.Report.Summary)
	fmt.Printf("trace_llm_calls=%d prompt_renders=%d structured_outputs=%d\n", len(result.Trace.LLMCalls), len(result.Trace.PromptRenders), len(result.Trace.StructuredOutputs))
}

func failf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
