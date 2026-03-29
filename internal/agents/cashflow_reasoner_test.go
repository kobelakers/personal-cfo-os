package agents

import (
	"testing"
	"time"

	contextview "github.com/kobelakers/personal-cfo-os/internal/context"
	"github.com/kobelakers/personal-cfo-os/internal/memory"
	"github.com/kobelakers/personal-cfo-os/internal/model"
	"github.com/kobelakers/personal-cfo-os/internal/observability"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/planning"
	"github.com/kobelakers/personal-cfo-os/internal/prompt"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/tools"
)

func TestProviderBackedCashflowReasonerHappyPath(t *testing.T) {
	registry, err := prompt.NewRegistry()
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	promptTrace := &observability.PromptTraceLog{}
	structuredTrace := &observability.StructuredOutputTraceLog{}
	reasoner := ProviderBackedCashflowReasoner{
		Base: DeterministicCashflowReasoner{MetricsTool: tools.ComputeCashflowMetricsTool{}},
		PromptRenderer: prompt.PromptRenderer{
			Registry:      registry,
			TraceRecorder: promptTrace,
		},
		Generator: model.DefaultStructuredGenerator{
			Model: model.StaticChatModel{
				Provider: "test-provider",
				Responder: func(request model.ModelRequest) (model.ModelResponse, error) {
					return model.ModelResponse{
						Provider: "test-provider",
						Model:    "fast-model",
						Profile:  request.Profile,
						Content: `{
							"summary":"模型确认本月现金流整体健康，但订阅和夜间消费仍需要继续跟踪。",
							"key_findings":["净结余为正。","重复订阅仍是主要可优化项。"],
							"grounded_recommendations":[{"title":"先清理低使用率订阅","detail":"selected evidence 已经包含订阅信号。","severity":"low","evidence_refs":["evidence-transaction-batch-user-1-20260329080000"]}],
							"risk_flags":[{"code":"cashflow_monitoring","severity":"low","detail":"建议继续跟踪波动支出。","evidence_ids":["evidence-transaction-batch-user-1-20260329080000"]}],
							"metric_refs":["monthly_net_income_cents","savings_rate","duplicate_subscription_count"],
							"evidence_refs":["evidence-transaction-batch-user-1-20260329080000"],
							"confidence":0.82,
							"caveats":["所有金额与比率仍以 deterministic metrics 为准。"]
						}`,
						Usage: model.UsageStats{
							PromptTokens:     90,
							CompletionTokens: 70,
							TotalTokens:      160,
							EstimatedCostUSD: 0.001,
						},
						Latency: 10 * time.Millisecond,
					}, nil
				},
			},
		},
		TraceRecorder: structuredTrace,
	}

	result, err := reasoner.Analyze(t.Context(), sampleCashflowReasonerInput())
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	if result.Summary == "" || len(result.KeyFindings) == 0 {
		t.Fatalf("expected provider-backed cashflow summary/findings, got %+v", result)
	}
	if result.DeterministicMetrics.MonthlyNetIncomeCents != 644900 {
		t.Fatalf("expected deterministic metrics to remain source of truth, got %+v", result.DeterministicMetrics)
	}
	if len(result.MetricRefs) == 0 || len(result.Caveats) == 0 {
		t.Fatalf("expected metric refs and caveats, got %+v", result)
	}
	if len(promptTrace.Records()) != 1 {
		t.Fatalf("expected one prompt trace, got %+v", promptTrace.Records())
	}
	if len(structuredTrace.Records()) != 1 || structuredTrace.Records()[0].FallbackUsed {
		t.Fatalf("expected one non-fallback structured trace, got %+v", structuredTrace.Records())
	}
}

func TestProviderBackedCashflowReasonerFallsBackOnGroundingFailure(t *testing.T) {
	registry, err := prompt.NewRegistry()
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	structuredTrace := &observability.StructuredOutputTraceLog{}
	reasoner := ProviderBackedCashflowReasoner{
		Base: DeterministicCashflowReasoner{MetricsTool: tools.ComputeCashflowMetricsTool{}},
		PromptRenderer: prompt.PromptRenderer{
			Registry: registry,
		},
		Generator: model.DefaultStructuredGenerator{
			Model: model.StaticChatModel{
				Provider: "test-provider",
				Responder: func(request model.ModelRequest) (model.ModelResponse, error) {
					return model.ModelResponse{
						Provider: "test-provider",
						Model:    "fast-model",
						Profile:  request.Profile,
						Content: `{
							"summary":"这是一条未通过 grounding 的结果。",
							"key_findings":["模型引用了未选中的证据。"],
							"grounded_recommendations":[{"title":"错误建议","detail":"引用了不在 selected evidence 中的证据。","severity":"low","evidence_refs":["evidence-not-selected"]}],
							"risk_flags":[{"code":"cashflow_monitoring","severity":"low","detail":"引用了无效证据。","evidence_ids":["evidence-not-selected"]}],
							"metric_refs":["monthly_net_income_cents"],
							"evidence_refs":["evidence-not-selected"],
							"confidence":0.51,
							"caveats":["这条结果应该被 grounding 拦下。"]
						}`,
						Usage: model.UsageStats{
							PromptTokens:     90,
							CompletionTokens: 70,
							TotalTokens:      160,
							EstimatedCostUSD: 0.001,
						},
						Latency: 10 * time.Millisecond,
					}, nil
				},
			},
		},
		TraceRecorder: structuredTrace,
	}

	result, err := reasoner.Analyze(t.Context(), sampleCashflowReasonerInput())
	if err != nil {
		t.Fatalf("analyze with fallback: %v", err)
	}
	if result.Summary == "这是一条未通过 grounding 的结果。" {
		t.Fatalf("expected deterministic fallback result, got %+v", result)
	}
	if len(result.Caveats) == 0 || result.Caveats[len(result.Caveats)-1] == "" {
		t.Fatalf("expected fallback caveat, got %+v", result.Caveats)
	}
}

func TestProviderBackedCashflowReasonerRepairSuccessRecordsRepairTrace(t *testing.T) {
	registry, err := prompt.NewRegistry()
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	structuredTrace := &observability.StructuredOutputTraceLog{}
	callLog := &cashflowCallRecorder{}
	usageLog := &cashflowUsageRecorder{}
	reasoner := ProviderBackedCashflowReasoner{
		Base:           DeterministicCashflowReasoner{MetricsTool: tools.ComputeCashflowMetricsTool{}},
		PromptRenderer: prompt.PromptRenderer{Registry: registry},
		Generator: model.DefaultStructuredGenerator{
			Model: &model.ScriptedChatModel{
				Steps: []model.ScriptedChatStep{
					{
						ExpectPromptIDPrefix: "cashflow.monthly_review.v1",
						ExpectPhase:          model.GenerationPhaseInitial,
						Response:             model.ModelResponse{Content: `{"summary":`},
					},
					{
						ExpectPromptIDPrefix: "cashflow.monthly_review.v1.repair",
						ExpectPhase:          model.GenerationPhaseRepair,
						Response: model.ModelResponse{
							Content: `{
								"summary":"本月现金流整体稳定，数字引用与 deterministic metrics 保持一致。",
								"key_findings":["月度净结余为 644900 分。","储蓄率 0.81，当前缓冲仍然充足。"],
								"grounded_recommendations":[{"title":"优先清理 2 个重复订阅","detail":"重复订阅 2 个，先清理低使用率项目。","severity":"low","evidence_refs":["evidence-transaction-batch-user-1-20260329080000"]}],
								"risk_flags":[{"code":"late_night_spending","severity":"low","detail":"深夜消费频率 0.12，建议继续观察。","evidence_ids":["evidence-transaction-batch-user-1-20260329080000"]}],
								"metric_refs":["monthly_net_income_cents","savings_rate","duplicate_subscription_count","late_night_spending_frequency"],
								"evidence_refs":["evidence-transaction-batch-user-1-20260329080000"],
								"confidence":0.82,
								"caveats":["所有金额与比率仍以 deterministic metrics 为准。"]
							}`,
						},
					},
				},
				CallRecorder:  callLog,
				UsageRecorder: usageLog,
			},
		},
		TraceRecorder: structuredTrace,
	}

	result, err := reasoner.Analyze(t.Context(), sampleCashflowReasonerInput())
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	if result.Summary == "" || len(result.KeyFindings) == 0 {
		t.Fatalf("expected repaired cashflow result, got %+v", result)
	}
	if len(structuredTrace.Records()) != 2 {
		t.Fatalf("expected initial + repair structured traces, got %+v", structuredTrace.Records())
	}
	last := structuredTrace.Records()[1]
	if last.GenerationPhase != model.GenerationPhaseRepair || last.PromptID != "cashflow.monthly_review.v1.repair" {
		t.Fatalf("expected repair structured trace, got %+v", last)
	}
	if len(callLog.records) != 2 || callLog.records[1].PromptID != "cashflow.monthly_review.v1.repair" {
		t.Fatalf("expected repair call trace, got %+v", callLog.records)
	}
	if len(usageLog.records) != 2 || usageLog.records[1].PromptID != "cashflow.monthly_review.v1.repair" {
		t.Fatalf("expected repair usage trace, got %+v", usageLog.records)
	}
}

func TestProviderBackedCashflowReasonerFallsBackWhenRepairStillInvalid(t *testing.T) {
	registry, err := prompt.NewRegistry()
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	structuredTrace := &observability.StructuredOutputTraceLog{}
	reasoner := ProviderBackedCashflowReasoner{
		Base:           DeterministicCashflowReasoner{MetricsTool: tools.ComputeCashflowMetricsTool{}},
		PromptRenderer: prompt.PromptRenderer{Registry: registry},
		Generator: model.DefaultStructuredGenerator{
			Model: &model.ScriptedChatModel{
				Steps: []model.ScriptedChatStep{
					{
						ExpectPromptIDPrefix: "cashflow.monthly_review.v1",
						ExpectPhase:          model.GenerationPhaseInitial,
						Response:             model.ModelResponse{Content: `{"summary":`},
					},
					{
						ExpectPromptIDPrefix: "cashflow.monthly_review.v1.repair",
						ExpectPhase:          model.GenerationPhaseRepair,
						Response:             model.ModelResponse{Content: `{"summary":`},
					},
				},
			},
		},
		TraceRecorder: structuredTrace,
	}

	result, err := reasoner.Analyze(t.Context(), sampleCashflowReasonerInput())
	if err != nil {
		t.Fatalf("analyze with fallback: %v", err)
	}
	if result.Summary == "" || len(result.Caveats) == 0 {
		t.Fatalf("expected deterministic fallback result, got %+v", result)
	}
	records := structuredTrace.Records()
	if len(records) < 3 {
		t.Fatalf("expected repair attempt and fallback trace, got %+v", records)
	}
	last := records[len(records)-1]
	if last.GenerationPhase != model.GenerationPhaseRepair || !last.FallbackUsed || last.FallbackReason == "" {
		t.Fatalf("expected repair-phase fallback trace, got %+v", last)
	}
}

func TestProviderBackedCashflowReasonerFallsBackOnUnsupportedNumericClaim(t *testing.T) {
	registry, err := prompt.NewRegistry()
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	structuredTrace := &observability.StructuredOutputTraceLog{}
	reasoner := ProviderBackedCashflowReasoner{
		Base:           DeterministicCashflowReasoner{MetricsTool: tools.ComputeCashflowMetricsTool{}},
		PromptRenderer: prompt.PromptRenderer{Registry: registry},
		Generator: model.DefaultStructuredGenerator{
			Model: model.StaticChatModel{
				Provider: "test-provider",
				Responder: func(request model.ModelRequest) (model.ModelResponse, error) {
					return model.ModelResponse{
						Provider: "test-provider",
						Model:    "fast-model",
						Profile:  request.Profile,
						Content: `{
							"summary":"本月净结余为 700000 分，现金流依然可控。",
							"key_findings":["重复订阅仍是主要可优化项。"],
							"grounded_recommendations":[{"title":"先清理低使用率订阅","detail":"selected evidence 已经包含订阅信号。","severity":"low","evidence_refs":["evidence-transaction-batch-user-1-20260329080000"]}],
							"risk_flags":[{"code":"cashflow_monitoring","severity":"low","detail":"建议继续跟踪波动支出。","evidence_ids":["evidence-transaction-batch-user-1-20260329080000"]}],
							"metric_refs":["monthly_net_income_cents"],
							"evidence_refs":["evidence-transaction-batch-user-1-20260329080000"],
							"confidence":0.82,
							"caveats":["所有金额与比率仍以 deterministic metrics 为准。"]
						}`,
					}, nil
				},
			},
		},
		TraceRecorder: structuredTrace,
	}

	result, err := reasoner.Analyze(t.Context(), sampleCashflowReasonerInput())
	if err != nil {
		t.Fatalf("analyze with fallback: %v", err)
	}
	if result.Summary == "本月净结余为 700000 分，现金流依然可控。" {
		t.Fatalf("expected unsupported numeric claim to force fallback, got %+v", result)
	}
	if len(result.Caveats) == 0 {
		t.Fatalf("expected fallback caveat, got %+v", result.Caveats)
	}
}

func TestProviderBackedCashflowReasonerFallsBackOnMetricSpecificNumericMismatch(t *testing.T) {
	registry, err := prompt.NewRegistry()
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	reasoner := ProviderBackedCashflowReasoner{
		Base:           DeterministicCashflowReasoner{MetricsTool: tools.ComputeCashflowMetricsTool{}},
		PromptRenderer: prompt.PromptRenderer{Registry: registry},
		Generator: model.DefaultStructuredGenerator{
			Model: model.StaticChatModel{
				Provider: "test-provider",
				Responder: func(request model.ModelRequest) (model.ModelResponse, error) {
					return model.ModelResponse{
						Provider: "test-provider",
						Model:    "fast-model",
						Profile:  request.Profile,
						Content: `{
							"summary":"本月流入 644900 分，整体可控。",
							"key_findings":["储蓄率 0.81，当前缓冲仍然充足。"],
							"grounded_recommendations":[{"title":"先清理低使用率订阅","detail":"selected evidence 已经包含订阅信号。","severity":"low","evidence_refs":["evidence-transaction-batch-user-1-20260329080000"]}],
							"risk_flags":[{"code":"cashflow_monitoring","severity":"low","detail":"建议继续跟踪波动支出。","evidence_ids":["evidence-transaction-batch-user-1-20260329080000"]}],
							"metric_refs":["monthly_inflow_cents","savings_rate"],
							"evidence_refs":["evidence-transaction-batch-user-1-20260329080000"],
							"confidence":0.82,
							"caveats":["所有金额与比率仍以 deterministic metrics 为准。"]
						}`,
					}, nil
				},
			},
		},
	}

	result, err := reasoner.Analyze(t.Context(), sampleCashflowReasonerInput())
	if err != nil {
		t.Fatalf("analyze with fallback: %v", err)
	}
	if result.Summary == "本月流入 644900 分，整体可控。" {
		t.Fatalf("expected metric-specific mismatch to force fallback, got %+v", result)
	}
}

func sampleCashflowReasonerInput() CashflowReasonerInput {
	evidenceID := observation.EvidenceID("evidence-transaction-batch-user-1-20260329080000")
	return CashflowReasonerInput{
		WorkflowID: "workflow-monthly-review",
		TaskID:     "task-monthly-review-20260329",
		TraceID:    "workflow-monthly-review",
		CurrentState: state.FinancialWorldState{
			UserID: "user-1",
			CashflowState: state.CashflowState{
				MonthlyInflowCents:    800000,
				MonthlyOutflowCents:   155100,
				MonthlyNetIncomeCents: 644900,
				SavingsRate:           0.81,
			},
			BehaviorState: state.BehaviorState{
				DuplicateSubscriptionCount: 2,
				LateNightSpendingFrequency: 0.12,
			},
		},
		RelevantMemories: []memory.MemoryRecord{
			{
				ID:         "memory-subscription",
				Kind:       memory.MemoryKindSemantic,
				Summary:    "subscription cleanup should be revisited",
				Confidence: memory.MemoryConfidence{Score: 0.9, Rationale: "direct"},
			},
		},
		RelevantEvidence: []observation.EvidenceRecord{
			{
				ID:      evidenceID,
				Type:    observation.EvidenceTypeTransactionBatch,
				Summary: "本月交易批次与订阅信号",
				Source: observation.EvidenceSource{
					Kind: "ledger", Adapter: "test", Reference: "tx-1", Provenance: "fixture",
				},
				TimeRange:     observation.EvidenceTimeRange{ObservedAt: time.Date(2026, 3, 29, 8, 0, 0, 0, time.UTC)},
				Confidence:    observation.EvidenceConfidence{Score: 0.9, Reason: "fixture"},
				Normalization: observation.EvidenceNormalizationResult{Status: observation.EvidenceNormalizationNormalized},
			},
		},
		Block: planning.ExecutionBlock{
			ID:                "cashflow-review",
			Kind:              planning.ExecutionBlockKindCashflowReview,
			AssignedRecipient: planning.BlockRecipientCashflowAgent,
			Goal:              "分析本月现金流并给出 grounded 建议。",
		},
		ExecutionContext: contextview.BlockExecutionContext{
			View:                 contextview.ContextViewExecution,
			BlockID:              "cashflow-review",
			BlockKind:            string(planning.ExecutionBlockKindCashflowReview),
			AssignedRecipient:    planning.BlockRecipientCashflowAgent,
			Goal:                 "分析本月现金流并给出 grounded 建议。",
			SelectedEvidenceIDs:  []observation.EvidenceID{evidenceID},
			SelectedMemoryIDs:    []string{"memory-subscription"},
			SelectedStateBlocks:  []string{"cashflow_state", "behavior_state"},
			EstimatedInputTokens: 320,
			Slice: contextview.ContextSlice{
				View:        contextview.ContextViewExecution,
				TaskID:      "cashflow-review",
				Goal:        "分析本月现金流并给出 grounded 建议。",
				MemoryIDs:   []string{"memory-subscription"},
				EvidenceIDs: []observation.EvidenceID{evidenceID},
				BudgetDecision: contextview.ContextBudgetDecision{
					EstimatedInputTokens: 320,
				},
			},
		},
	}
}

type cashflowCallRecorder struct {
	records []model.CallRecord
}

func (r *cashflowCallRecorder) RecordCall(record model.CallRecord) {
	r.records = append(r.records, record)
}

type cashflowUsageRecorder struct {
	records []model.UsageRecord
}

func (r *cashflowUsageRecorder) RecordUsage(record model.UsageRecord) {
	r.records = append(r.records, record)
}
