package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kobelakers/personal-cfo-os/internal/model"
	"github.com/kobelakers/personal-cfo-os/internal/state"
)

func TestMonthlyReview5BLiveSmoke(t *testing.T) {
	if os.Getenv("RUN_MONTHLY_REVIEW_5B_LIVE_SMOKE") == "" {
		t.Skip("set RUN_MONTHLY_REVIEW_5B_LIVE_SMOKE=1 to enable live provider smoke")
	}
	if os.Getenv("OPENAI_API_KEY") == "" {
		t.Skip("OPENAI_API_KEY is required for live provider smoke")
	}

	env, err := OpenMonthlyReview5BEnvironment(MonthlyReview5BOptions{
		FixtureDir:      monthlyReview5BFixtureDir(),
		HoldingsFixture: "holdings_2026-03-safe.csv",
		Now:             fixedMonthlyReview5BNow,
		ChatModelFactory: func(callRecorder model.CallRecorder, usageRecorder model.UsageRecorder) model.ChatModel {
			return NewLiveMonthlyReviewChatModel(callRecorder, usageRecorder)
		},
	})
	if err != nil {
		t.Fatalf("open live monthly review environment: %v", err)
	}

	result, err := env.Run(context.Background(), "user-1", "请帮我做一份月度财务复盘", state.FinancialWorldState{})
	if err != nil {
		t.Fatalf("run live monthly review: %v", err)
	}
	if len(result.Trace.LLMCalls) < 2 || len(result.Trace.Usage) < 2 {
		t.Fatalf("expected live provider call and usage traces, got %+v", result.Trace)
	}
	outDir := t.TempDir()
	artifactPath := filepath.Join(outDir, "monthly_review_5b_live_report.json")
	tracePath := filepath.Join(outDir, "monthly_review_5b_live_trace.json")
	if err := result.WriteArtifact(artifactPath); err != nil {
		t.Fatalf("write live artifact: %v", err)
	}
	if err := result.WriteTrace(tracePath); err != nil {
		t.Fatalf("write live trace: %v", err)
	}
}
