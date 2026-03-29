package structured

import (
	"context"
	"errors"
	"testing"

	"github.com/kobelakers/personal-cfo-os/internal/model"
)

func TestPipelineFallsBackAfterMalformedOutput(t *testing.T) {
	pipeline := Pipeline[struct {
		Value string `json:"value"`
	}]{
		Schema: Schema[struct {
			Value string `json:"value"`
		}]{
			Name: "test_schema",
			Parser: JSONParser[struct {
				Value string `json:"value"`
			}]{},
			Validator: ValidatorFunc[struct {
				Value string `json:"value"`
			}](func(v struct {
				Value string `json:"value"`
			}) []string {
				return nil
			}),
		},
		Generator: model.DefaultStructuredGenerator{
			Model: model.StaticChatModel{
				Responder: func(request model.ModelRequest) (model.ModelResponse, error) {
					return model.ModelResponse{Content: "{not-json"}, nil
				},
			},
		},
		FallbackPolicy: FallbackPolicy[struct {
			Value string `json:"value"`
		}]{
			Name: "fallback",
			Execute: func() (struct {
				Value string `json:"value"`
			}, error) {
				return struct {
					Value string `json:"value"`
				}{Value: "fallback"}, nil
			},
		},
	}
	result, err := pipeline.Execute(context.Background(), model.StructuredGenerationRequest{
		ModelRequest: model.ModelRequest{PromptID: "test"},
	})
	if err != nil {
		t.Fatalf("pipeline execute: %v", err)
	}
	if !result.FallbackUsed || result.Value.Value != "fallback" {
		t.Fatalf("expected fallback result, got %+v", result)
	}
}

func TestPipelineReturnsErrorWhenFallbackFails(t *testing.T) {
	pipeline := Pipeline[struct {
		Value string `json:"value"`
	}]{
		Schema: Schema[struct {
			Value string `json:"value"`
		}]{
			Name: "test_schema",
			Parser: JSONParser[struct {
				Value string `json:"value"`
			}]{},
			Validator: ValidatorFunc[struct {
				Value string `json:"value"`
			}](func(v struct {
				Value string `json:"value"`
			}) []string {
				return nil
			}),
		},
		Generator: model.DefaultStructuredGenerator{
			Model: model.StaticChatModel{
				Responder: func(request model.ModelRequest) (model.ModelResponse, error) {
					return model.ModelResponse{}, errors.New("transport failed")
				},
			},
		},
		FallbackPolicy: FallbackPolicy[struct {
			Value string `json:"value"`
		}]{
			Name: "fallback",
			Execute: func() (struct {
				Value string `json:"value"`
			}, error) {
				return struct {
					Value string `json:"value"`
				}{}, errors.New("fallback failed")
			},
		},
	}
	if _, err := pipeline.Execute(context.Background(), model.StructuredGenerationRequest{
		ModelRequest: model.ModelRequest{PromptID: "test"},
	}); err == nil {
		t.Fatalf("expected pipeline failure when fallback also fails")
	}
}

func TestPipelineRepairSuccessPreservesRepairPromptIdentityInTrace(t *testing.T) {
	traceLog := &structuredTraceRecorder{}
	callLog := &callRecorder{}
	usageLog := &usageRecorder{}
	pipeline := Pipeline[testStructuredValue]{
		Schema: Schema[testStructuredValue]{
			Name:      "test_schema",
			Parser:    JSONParser[testStructuredValue]{},
			Validator: ValidatorFunc[testStructuredValue](func(v testStructuredValue) []string { return nil }),
		},
		Generator: model.DefaultStructuredGenerator{
			Model: &model.ScriptedChatModel{
				Steps: []model.ScriptedChatStep{
					{
						ExpectPromptIDPrefix: "planner.monthly_review.v1",
						ExpectPhase:          model.GenerationPhaseInitial,
						Response:             model.ModelResponse{Content: `{"value":`},
					},
					{
						ExpectPromptIDPrefix: "planner.monthly_review.v1.repair",
						ExpectPhase:          model.GenerationPhaseRepair,
						Response:             model.ModelResponse{Content: `{"value":"repaired"}`},
					},
				},
				CallRecorder:  callLog,
				UsageRecorder: usageLog,
			},
		},
		TraceRecorder: traceLog,
		FallbackPolicy: FallbackPolicy[testStructuredValue]{
			Name: "fallback",
			Execute: func() (testStructuredValue, error) {
				return testStructuredValue{Value: "fallback"}, nil
			},
		},
	}

	result, err := pipeline.Execute(context.Background(), model.StructuredGenerationRequest{
		ModelRequest: model.ModelRequest{
			Profile:       model.ModelProfilePlannerReasoning,
			PromptID:      "planner.monthly_review.v1",
			PromptVersion: "v1",
		},
	})
	if err != nil {
		t.Fatalf("pipeline execute: %v", err)
	}
	if result.Value.Value != "repaired" || result.RepairAttempts != 1 {
		t.Fatalf("expected repaired result, got %+v", result)
	}
	if len(traceLog.records) != 2 {
		t.Fatalf("expected initial + repair structured traces, got %+v", traceLog.records)
	}
	if traceLog.records[1].GenerationPhase != model.GenerationPhaseRepair || traceLog.records[1].PromptID != "planner.monthly_review.v1.repair" {
		t.Fatalf("expected repair trace identity, got %+v", traceLog.records[1])
	}
	if len(callLog.records) != 2 || callLog.records[1].PromptID != "planner.monthly_review.v1.repair" || callLog.records[1].GenerationPhase != model.GenerationPhaseRepair {
		t.Fatalf("expected repair call record identity, got %+v", callLog.records)
	}
	if len(usageLog.records) != 2 || usageLog.records[1].PromptID != "planner.monthly_review.v1.repair" || usageLog.records[1].GenerationPhase != model.GenerationPhaseRepair {
		t.Fatalf("expected repair usage record identity, got %+v", usageLog.records)
	}
}

func TestPipelineRepairsSchemaInvalidOutputAndPreservesRepairTrace(t *testing.T) {
	traceLog := &structuredTraceRecorder{}
	pipeline := Pipeline[testStructuredValue]{
		Schema: Schema[testStructuredValue]{
			Name:   "test_schema",
			Parser: JSONParser[testStructuredValue]{},
			Validator: ValidatorFunc[testStructuredValue](func(v testStructuredValue) []string {
				if v.Value != "ok" {
					return []string{"value must be ok"}
				}
				return nil
			}),
		},
		Generator: model.DefaultStructuredGenerator{
			Model: &model.ScriptedChatModel{
				Steps: []model.ScriptedChatStep{
					{
						ExpectPromptIDPrefix: "cashflow.monthly_review.v1",
						ExpectPhase:          model.GenerationPhaseInitial,
						Response:             model.ModelResponse{Content: `{"value":"bad"}`},
					},
					{
						ExpectPromptIDPrefix: "cashflow.monthly_review.v1.repair",
						ExpectPhase:          model.GenerationPhaseRepair,
						Response:             model.ModelResponse{Content: `{"value":"ok"}`},
					},
				},
			},
		},
		TraceRecorder: traceLog,
		FallbackPolicy: FallbackPolicy[testStructuredValue]{
			Name: "fallback",
			Execute: func() (testStructuredValue, error) {
				return testStructuredValue{Value: "fallback"}, nil
			},
		},
	}

	result, err := pipeline.Execute(context.Background(), model.StructuredGenerationRequest{
		ModelRequest: model.ModelRequest{
			Profile:       model.ModelProfileCashflowFast,
			PromptID:      "cashflow.monthly_review.v1",
			PromptVersion: "v1",
		},
	})
	if err != nil {
		t.Fatalf("pipeline execute: %v", err)
	}
	if result.Value.Value != "ok" || result.RepairAttempts != 1 {
		t.Fatalf("expected repaired schema-valid result, got %+v", result)
	}
	if len(traceLog.records) != 2 {
		t.Fatalf("expected initial + repair structured traces, got %+v", traceLog.records)
	}
	if traceLog.records[0].FailureCategory != FailureCategorySchemaInvalid {
		t.Fatalf("expected initial schema-invalid trace, got %+v", traceLog.records[0])
	}
	if traceLog.records[1].PromptID != "cashflow.monthly_review.v1.repair" || traceLog.records[1].PromptVersion != "v1" {
		t.Fatalf("expected repair prompt identity in trace, got %+v", traceLog.records[1])
	}
}

func TestPipelineRepairFailureStillRecordsRepairAttemptBeforeFallback(t *testing.T) {
	traceLog := &structuredTraceRecorder{}
	pipeline := Pipeline[testStructuredValue]{
		Schema: Schema[testStructuredValue]{
			Name:      "test_schema",
			Parser:    JSONParser[testStructuredValue]{},
			Validator: ValidatorFunc[testStructuredValue](func(v testStructuredValue) []string { return nil }),
		},
		Generator: model.DefaultStructuredGenerator{
			Model: &model.ScriptedChatModel{
				Steps: []model.ScriptedChatStep{
					{
						ExpectPromptIDPrefix: "planner.monthly_review.v1",
						ExpectPhase:          model.GenerationPhaseInitial,
						Response:             model.ModelResponse{Content: `{"value":`},
					},
					{
						ExpectPromptIDPrefix: "planner.monthly_review.v1.repair",
						ExpectPhase:          model.GenerationPhaseRepair,
						Response:             model.ModelResponse{Content: `{"value":`},
					},
				},
			},
		},
		TraceRecorder: traceLog,
		FallbackPolicy: FallbackPolicy[testStructuredValue]{
			Name: "fallback",
			Execute: func() (testStructuredValue, error) {
				return testStructuredValue{Value: "fallback"}, nil
			},
		},
	}

	result, err := pipeline.Execute(context.Background(), model.StructuredGenerationRequest{
		ModelRequest: model.ModelRequest{
			Profile:       model.ModelProfilePlannerReasoning,
			PromptID:      "planner.monthly_review.v1",
			PromptVersion: "v1",
		},
	})
	if err != nil {
		t.Fatalf("pipeline execute: %v", err)
	}
	if !result.FallbackUsed || result.Value.Value != "fallback" {
		t.Fatalf("expected fallback after failed repair, got %+v", result)
	}
	if len(traceLog.records) < 3 {
		t.Fatalf("expected initial trace, repair trace, and fallback trace, got %+v", traceLog.records)
	}
	last := traceLog.records[len(traceLog.records)-1]
	if last.GenerationPhase != model.GenerationPhaseRepair || !last.FallbackUsed || last.PromptID != "planner.monthly_review.v1.repair" {
		t.Fatalf("expected repair-phase fallback trace, got %+v", last)
	}
}

type testStructuredValue struct {
	Value string `json:"value"`
}

type structuredTraceRecorder struct {
	records []TraceRecord
}

func (r *structuredTraceRecorder) RecordStructuredOutput(record TraceRecord) {
	r.records = append(r.records, record)
}

type callRecorder struct {
	records []model.CallRecord
}

func (r *callRecorder) RecordCall(record model.CallRecord) {
	r.records = append(r.records, record)
}

type usageRecorder struct {
	records []model.UsageRecord
}

func (r *usageRecorder) RecordUsage(record model.UsageRecord) {
	r.records = append(r.records, record)
}
