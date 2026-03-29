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
	records := structuredTrace.Records()
	if len(records) == 0 || !records[len(records)-1].FallbackUsed {
		t.Fatalf("expected structured fallback trace, got %+v", records)
	}
}

func TestProviderBackedPlannerRepairSuccessRecordsRepairTrace(t *testing.T) {
	registry, err := prompt.NewRegistry()
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	structuredTrace := &observability.StructuredOutputTraceLog{}
	callLog := &modelCallRecorder{}
	usageLog := &modelUsageRecorder{}
	planner := ProviderBackedPlanner{
		PromptRenderer: prompt.PromptRenderer{Registry: registry},
		Generator: model.DefaultStructuredGenerator{
			Model: &model.ScriptedChatModel{
				Steps: []model.ScriptedChatStep{
					{
						ExpectPromptIDPrefix: "planner.monthly_review.v1",
						ExpectPhase:          model.GenerationPhaseInitial,
						Response:             model.ModelResponse{Content: `{"plan_summary":`},
					},
					{
						ExpectPromptIDPrefix: "planner.monthly_review.v1.repair",
						ExpectPhase:          model.GenerationPhaseRepair,
						Response: model.ModelResponse{
							Content: `{
								"plan_summary":"repair fixed the plan output",
								"rationale":"repair preserved block order",
								"verification_focus_notes":["cashflow_grounding","debt_grounding"],
								"block_order":["cashflow-review","debt-review"],
								"step_emphasis":[{"step_id":"compute-metrics","emphasis":"repair kept the focus on deterministic metrics."}]
							}`,
						},
					},
				},
				CallRecorder:  callLog,
				UsageRecorder: usageLog,
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
	if plan.Trace == nil || plan.Trace.FallbackUsed {
		t.Fatalf("expected repair success without fallback, got %+v", plan.Trace)
	}
	if len(structuredTrace.Records()) != 2 {
		t.Fatalf("expected initial + repair structured trace, got %+v", structuredTrace.Records())
	}
	lastTrace := structuredTrace.Records()[1]
	if lastTrace.GenerationPhase != model.GenerationPhaseRepair || lastTrace.PromptID != "planner.monthly_review.v1.repair" {
		t.Fatalf("expected repair prompt identity in structured trace, got %+v", lastTrace)
	}
	if len(callLog.records) != 2 || callLog.records[1].PromptID != "planner.monthly_review.v1.repair" {
		t.Fatalf("expected repair call trace, got %+v", callLog.records)
	}
	if len(usageLog.records) != 2 || usageLog.records[1].PromptID != "planner.monthly_review.v1.repair" {
		t.Fatalf("expected repair usage trace, got %+v", usageLog.records)
	}
}

func TestProviderBackedPlannerRepairFailureFallsBackAndRecordsRepairAttempt(t *testing.T) {
	registry, err := prompt.NewRegistry()
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	structuredTrace := &observability.StructuredOutputTraceLog{}
	planner := ProviderBackedPlanner{
		PromptRenderer: prompt.PromptRenderer{Registry: registry},
		Generator: model.DefaultStructuredGenerator{
			Model: &model.ScriptedChatModel{
				Steps: []model.ScriptedChatStep{
					{
						ExpectPromptIDPrefix: "planner.monthly_review.v1",
						ExpectPhase:          model.GenerationPhaseInitial,
						Response:             model.ModelResponse{Content: `{"plan_summary":`},
					},
					{
						ExpectPromptIDPrefix: "planner.monthly_review.v1.repair",
						ExpectPhase:          model.GenerationPhaseRepair,
						Response:             model.ModelResponse{Content: `{"plan_summary":`},
					},
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
		t.Fatalf("expected planner fallback after failed repair, got %+v", plan.Trace)
	}
	records := structuredTrace.Records()
	if len(records) < 3 {
		t.Fatalf("expected fallback trace to retain repair attempt evidence, got %+v", records)
	}
	last := records[len(records)-1]
	if last.GenerationPhase != model.GenerationPhaseRepair || !last.FallbackUsed {
		t.Fatalf("expected repair-phase fallback trace, got %+v", last)
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

type modelCallRecorder struct {
	records []model.CallRecord
}

func (r *modelCallRecorder) RecordCall(record model.CallRecord) {
	r.records = append(r.records, record)
}

type modelUsageRecorder struct {
	records []model.UsageRecord
}

func (r *modelUsageRecorder) RecordUsage(record model.UsageRecord) {
	r.records = append(r.records, record)
}
