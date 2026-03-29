package planning

import (
	"testing"
	"time"

	contextview "github.com/kobelakers/personal-cfo-os/internal/context"
	"github.com/kobelakers/personal-cfo-os/internal/model"
	"github.com/kobelakers/personal-cfo-os/internal/observability"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/prompt"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

func TestProviderBackedPlannerHappyPath(t *testing.T) {
	registry, err := prompt.NewRegistry()
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	promptTrace := &observability.PromptTraceLog{}
	structuredTrace := &observability.StructuredOutputTraceLog{}
	planner := ProviderBackedPlanner{
		PromptRenderer: prompt.PromptRenderer{
			Registry:      registry,
			TraceRecorder: promptTrace,
		},
		Generator: model.DefaultStructuredGenerator{
			Model: model.StaticChatModel{
				Provider: "test-provider",
				Responder: func(request model.ModelRequest) (model.ModelResponse, error) {
					return model.ModelResponse{
						Provider: request.Provider,
						Model:    "reasoning-test",
						Profile:  request.Profile,
						Content: `{
							"plan_summary":"先处理债务压力，再复核现金流缓冲。",
							"rationale":"债务块风险更高，应先看 debt-review，再看 cashflow-review。",
							"verification_focus_notes":["debt_grounding","cashflow_grounding"],
							"block_order":["debt-review","cashflow-review"],
							"step_emphasis":[{"step_id":"compute-metrics","emphasis":"先确认债务负担率和最低还款压力。"}]
						}`,
						Usage: model.UsageStats{
							PromptTokens:     100,
							CompletionTokens: 60,
							TotalTokens:      160,
							EstimatedCostUSD: 0.001,
						},
						Latency: 10 * time.Millisecond,
					}, nil
				},
			},
		},
		TraceRecorder: structuredTrace,
		CatalogBuilder: CandidatePlanCatalogBuilder{
			Planner: &DeterministicPlanner{Now: fixedPlannerNow},
		},
		Compiler: PlanCompiler{},
		Fallback: DeterministicFallbackPlanner{
			Planner: &DeterministicPlanner{Now: fixedPlannerNow},
		},
		Now: fixedPlannerNow,
	}

	plan := planner.CreatePlan(sampleMonthlyReviewTaskSpec(), samplePlanningContextSlice(), "workflow-monthly-review")
	if plan.Summary == "" || plan.Rationale == "" {
		t.Fatalf("expected structured planner summary and rationale, got %+v", plan)
	}
	if len(plan.Blocks) != 2 || plan.Blocks[0].ID != "debt-review" {
		t.Fatalf("expected provider-backed block order, got %+v", plan.Blocks)
	}
	if plan.Trace == nil || plan.Trace.PromptID != "planner.monthly_review.v1" || plan.Trace.FallbackUsed {
		t.Fatalf("expected planner trace without fallback, got %+v", plan.Trace)
	}
	if len(promptTrace.Records()) != 1 {
		t.Fatalf("expected one prompt trace, got %+v", promptTrace.Records())
	}
	if len(structuredTrace.Records()) != 1 || structuredTrace.Records()[0].SchemaName != "PlannerPlanSchema" {
		t.Fatalf("expected one structured output trace, got %+v", structuredTrace.Records())
	}
}

func TestProviderBackedPlannerFallsBackWhenStructuredOutputIsMalformed(t *testing.T) {
	registry, err := prompt.NewRegistry()
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	structuredTrace := &observability.StructuredOutputTraceLog{}
	planner := ProviderBackedPlanner{
		PromptRenderer: prompt.PromptRenderer{Registry: registry},
		Generator: model.DefaultStructuredGenerator{
			Model: model.StaticChatModel{
				Provider: "test-provider",
				Responder: func(request model.ModelRequest) (model.ModelResponse, error) {
					return model.ModelResponse{
						Provider: "test-provider",
						Model:    "reasoning-test",
						Profile:  request.Profile,
						Content:  `{"broken_json":`,
						Usage: model.UsageStats{
							PromptTokens:     80,
							CompletionTokens: 40,
							TotalTokens:      120,
						},
						Latency: 10 * time.Millisecond,
					}, nil
				},
			},
		},
		TraceRecorder: structuredTrace,
		CatalogBuilder: CandidatePlanCatalogBuilder{
			Planner: &DeterministicPlanner{Now: fixedPlannerNow},
		},
		Compiler: PlanCompiler{},
		Fallback: DeterministicFallbackPlanner{
			Planner: &DeterministicPlanner{Now: fixedPlannerNow},
		},
		Now: fixedPlannerNow,
	}

	plan := planner.CreatePlan(sampleMonthlyReviewTaskSpec(), samplePlanningContextSlice(), "workflow-monthly-review")
	if plan.Trace == nil || !plan.Trace.FallbackUsed {
		t.Fatalf("expected planner fallback trace, got %+v", plan.Trace)
	}
	if plan.Summary == "" || plan.Trace.StructuredFailure == "" {
		t.Fatalf("expected fallback summary and structured failure, got %+v", plan)
	}
	if len(structuredTrace.Records()) != 1 || !structuredTrace.Records()[0].FallbackUsed {
		t.Fatalf("expected structured fallback trace, got %+v", structuredTrace.Records())
	}
}

func sampleMonthlyReviewTaskSpec() taskspec.TaskSpec {
	return taskspec.TaskSpec{
		ID:             "task-monthly-review-20260329",
		Goal:           "请帮我做一份月度财务复盘",
		UserIntentType: taskspec.UserIntentMonthlyReview,
	}
}

func samplePlanningContextSlice() contextview.ContextSlice {
	return contextview.ContextSlice{
		View:        contextview.ContextViewPlanning,
		TaskID:      "task-monthly-review-20260329",
		Goal:        "请帮我做一份月度财务复盘",
		MemoryIDs:   []string{"memory-1"},
		EvidenceIDs: []observation.EvidenceID{"evidence-1"},
		BudgetDecision: contextview.ContextBudgetDecision{
			EstimatedInputTokens: 256,
		},
	}
}

func fixedPlannerNow() time.Time {
	return time.Date(2026, 3, 29, 8, 0, 0, 0, time.UTC)
}
