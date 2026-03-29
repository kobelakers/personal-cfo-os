package app

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/model"
	"github.com/kobelakers/personal-cfo-os/internal/state"
)

func TestMonthlyReview5BMockGoldenPath(t *testing.T) {
	env, err := OpenMonthlyReview5BEnvironment(MonthlyReview5BOptions{
		FixtureDir:      monthlyReview5BFixtureDir(),
		HoldingsFixture: "holdings_2026-03-safe.csv",
		Now:             fixedMonthlyReview5BNow,
		ChatModelFactory: func(callRecorder model.CallRecorder, usageRecorder model.UsageRecorder) model.ChatModel {
			return NewMockMonthlyReviewChatModelWithTrace(callRecorder, usageRecorder)
		},
	})
	if err != nil {
		t.Fatalf("open monthly review 5b environment: %v", err)
	}

	result, err := env.Run(t.Context(), "user-1", "请帮我做一份月度财务复盘", state.FinancialWorldState{})
	if err != nil {
		t.Fatalf("run monthly review 5b: %v", err)
	}
	if result.Result.Report.Summary == "" {
		t.Fatalf("expected non-empty report summary")
	}
	if len(result.Trace.PromptRenders) != 2 || len(result.Trace.LLMCalls) != 2 || len(result.Trace.Usage) != 2 {
		t.Fatalf("expected prompt/provider/usage traces, got %+v", result.Trace)
	}
	if len(result.Trace.StructuredOutputs) != 2 {
		t.Fatalf("expected structured output traces, got %+v", result.Trace.StructuredOutputs)
	}
	if result.Trace.PromptRenders[0].PromptID == "" || result.Trace.PromptRenders[1].PromptVersion == "" {
		t.Fatalf("expected prompt version metadata, got %+v", result.Trace.PromptRenders)
	}
	if result.Trace.PromptRenders[0].AppliedPolicy == "" || result.Trace.PromptRenders[1].AppliedPolicy == "" {
		t.Fatalf("expected applied render policy metadata, got %+v", result.Trace.PromptRenders)
	}
	if result.Trace.PromptRenders[0].EstimatedInputTokens == result.Trace.PromptRenders[1].EstimatedInputTokens {
		t.Fatalf("expected planning and execution prompt traces to show different estimated token usage, got %+v", result.Trace.PromptRenders)
	}
	for _, call := range result.Trace.LLMCalls {
		if call.Agent == "" || call.PromptID == "" {
			t.Fatalf("expected llm call trace to include agent/prompt, got %+v", call)
		}
	}
	for _, usage := range result.Trace.Usage {
		if usage.TotalTokens <= 0 || usage.EstimatedCostUSD <= 0 {
			t.Fatalf("expected usage cost metadata, got %+v", usage)
		}
	}
	for _, item := range result.Trace.StructuredOutputs {
		if item.FallbackUsed {
			t.Fatalf("expected mock golden path without fallback, got %+v", result.Trace.StructuredOutputs)
		}
	}
}

func TestMonthlyReview5BFallbackPathStillProducesTraceableReport(t *testing.T) {
	env, err := OpenMonthlyReview5BEnvironment(MonthlyReview5BOptions{
		FixtureDir:      monthlyReview5BFixtureDir(),
		HoldingsFixture: "holdings_2026-03-safe.csv",
		Now:             fixedMonthlyReview5BNow,
		ChatModelFactory: func(callRecorder model.CallRecorder, usageRecorder model.UsageRecorder) model.ChatModel {
			return fallbackMonthlyReviewChatModel{
				callRecorder:  callRecorder,
				usageRecorder: usageRecorder,
			}
		},
	})
	if err != nil {
		t.Fatalf("open monthly review 5b environment: %v", err)
	}

	result, err := env.Run(context.Background(), "user-1", "请帮我做一份月度财务复盘", state.FinancialWorldState{})
	if err != nil {
		t.Fatalf("run monthly review with fallback: %v", err)
	}
	if result.Result.Report.Summary == "" {
		t.Fatalf("expected final report even after fallback")
	}
	fallbackSeen := false
	for _, item := range result.Trace.StructuredOutputs {
		if item.FallbackUsed {
			fallbackSeen = true
		}
	}
	if !fallbackSeen {
		t.Fatalf("expected fallback path to enter structured output trace, got %+v", result.Trace.StructuredOutputs)
	}
	if len(result.Trace.PromptRenders) == 0 || len(result.Trace.LLMCalls) == 0 {
		t.Fatalf("expected prompt/provider traces on fallback path, got %+v", result.Trace)
	}
}

func monthlyReview5BFixtureDir() string {
	return filepath.Join("..", "..", "tests", "fixtures")
}

func fixedMonthlyReview5BNow() time.Time {
	return time.Date(2026, 3, 29, 8, 0, 0, 0, time.UTC)
}

type fallbackMonthlyReviewChatModel struct {
	callRecorder  model.CallRecorder
	usageRecorder model.UsageRecorder
}

func (m fallbackMonthlyReviewChatModel) Generate(_ context.Context, request model.ModelRequest) (model.ModelResponse, error) {
	start := time.Now().UTC()
	response := model.ModelResponse{
		Provider: "fallback-test-provider",
		Model:    "fallback-test-model",
		Profile:  request.Profile,
		Content:  `{"broken_json":`,
		Usage: model.UsageStats{
			PromptTokens:     32,
			CompletionTokens: 16,
			TotalTokens:      48,
			EstimatedCostUSD: 0.0001,
		},
		Latency: 5 * time.Millisecond,
	}
	if m.callRecorder != nil {
		m.callRecorder.RecordCall(model.CallRecord{
			Provider:      response.Provider,
			Model:         response.Model,
			Profile:       request.Profile,
			WorkflowID:    request.WorkflowID,
			TaskID:        request.TaskID,
			TraceID:       request.TraceID,
			Agent:         request.Agent,
			PromptID:      request.PromptID,
			PromptVersion: request.PromptVersion,
			StartedAt:     start,
			CompletedAt:   time.Now().UTC(),
			LatencyMS:     response.Latency.Milliseconds(),
		})
	}
	if m.usageRecorder != nil {
		m.usageRecorder.RecordUsage(model.UsageRecord{
			Provider:         response.Provider,
			Model:            response.Model,
			Profile:          request.Profile,
			WorkflowID:       request.WorkflowID,
			TaskID:           request.TaskID,
			TraceID:          request.TraceID,
			Agent:            request.Agent,
			PromptID:         request.PromptID,
			PromptVersion:    request.PromptVersion,
			PromptTokens:     response.Usage.PromptTokens,
			CompletionTokens: response.Usage.CompletionTokens,
			TotalTokens:      response.Usage.TotalTokens,
			EstimatedCostUSD: response.Usage.EstimatedCostUSD,
			RecordedAt:       time.Now().UTC(),
		})
	}
	return response, nil
}
