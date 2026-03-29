package protocol

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/analysis"
	contextview "github.com/kobelakers/personal-cfo-os/internal/context"
	"github.com/kobelakers/personal-cfo-os/internal/memory"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/planning"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

func TestAgentEnvelopeRoundTripPreservesCorrelationChain(t *testing.T) {
	now := time.Date(2026, 3, 28, 12, 0, 0, 0, time.UTC)
	task := taskspec.TaskSpec{
		ID:   "task-1",
		Goal: "Run monthly review",
		Scope: taskspec.TaskScope{
			Areas: []string{"cashflow"},
		},
		Constraints: taskspec.ConstraintSet{Hard: []string{"no auto-execution"}},
		RiskLevel:   taskspec.RiskLevelMedium,
		SuccessCriteria: []taskspec.SuccessCriteria{
			{ID: "coverage", Description: "Required evidence covered"},
		},
		RequiredEvidence: []taskspec.RequiredEvidenceRef{
			{Type: "ledger_transaction", Reason: "monthly spending coverage", Mandatory: true},
		},
		ApprovalRequirement: taskspec.ApprovalRequirementRecommended,
		UserIntentType:      taskspec.UserIntentMonthlyReview,
		CreatedAt:           now,
	}
	envelope := AgentEnvelope{
		Metadata: ProtocolMetadata{
			MessageID:     "msg-1",
			CorrelationID: "corr-1",
			CausationID:   "cause-1",
			EmittedAt:     now,
		},
		Sender:           "planner-agent",
		Recipient:        "cashflow-agent",
		Task:             task,
		StateRef:         StateReference{UserID: "user-1", SnapshotID: "state-v2", Version: 2},
		RequiredEvidence: task.RequiredEvidence,
		Deadline:         &now,
		RiskLevel:        task.RiskLevel,
		Kind:             MessageKindPlanRequest,
		Payload: AgentRequestBody{
			PlanRequest: &PlanRequestPayload{
				CurrentState: state.FinancialWorldState{
					UserID:  "user-1",
					Version: state.StateVersion{Sequence: 2, SnapshotID: "state-v2", UpdatedAt: now},
				},
				Evidence:     []observation.EvidenceRecord{{ID: "ev-1", Type: observation.EvidenceTypeTransactionBatch}},
				PlanningView: contextview.ContextViewPlanning,
			},
		},
	}

	data, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	var decoded AgentEnvelope
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if decoded.Metadata.CorrelationID != envelope.Metadata.CorrelationID || decoded.Metadata.CausationID != envelope.Metadata.CausationID {
		t.Fatalf("correlation chain was not preserved: %+v", decoded.Metadata)
	}
	if err := decoded.Validate(); err != nil {
		t.Fatalf("decoded envelope should validate: %v", err)
	}
}

func TestAgentEnvelopeRejectsInvalidOneOfPayload(t *testing.T) {
	now := time.Date(2026, 3, 28, 12, 0, 0, 0, time.UTC)
	task := taskspec.DeterministicIntakeService{Now: func() time.Time { return now }}.Parse("请帮我做一份月度财务复盘")
	if task.TaskSpec == nil {
		t.Fatalf("expected monthly review task spec")
	}
	envelope := AgentEnvelope{
		Metadata: ProtocolMetadata{
			MessageID:     "msg-bad",
			CorrelationID: "corr-bad",
			CausationID:   "cause-bad",
			EmittedAt:     now,
		},
		Sender:           "workflow",
		Recipient:        "planner_agent",
		Task:             *task.TaskSpec,
		StateRef:         StateReference{UserID: "user-1", SnapshotID: "snap-1", Version: 1},
		RequiredEvidence: task.TaskSpec.RequiredEvidence,
		Deadline:         task.TaskSpec.Deadline,
		RiskLevel:        task.TaskSpec.RiskLevel,
		Kind:             MessageKindPlanRequest,
		Payload: AgentRequestBody{
			PlanRequest: &PlanRequestPayload{
				CurrentState: state.FinancialWorldState{
					UserID:  "user-1",
					Version: state.StateVersion{Sequence: 1, SnapshotID: "snap-1", UpdatedAt: now},
				},
				Evidence:     []observation.EvidenceRecord{validEvidence(now, "ev-1", observation.EvidenceTypeTransactionBatch, "transaction_batch")},
				PlanningView: contextview.ContextViewPlanning,
			},
			MemorySyncRequest: &MemorySyncRequestPayload{
				CurrentState: state.FinancialWorldState{
					UserID:  "user-1",
					Version: state.StateVersion{Sequence: 1, SnapshotID: "snap-1", UpdatedAt: now},
				},
				Evidence: []observation.EvidenceRecord{validEvidence(now, "ev-2", observation.EvidenceTypeDebtObligationSnapshot, "debt_obligation_snapshot")},
			},
		},
	}
	if err := envelope.Validate(); err == nil {
		t.Fatalf("expected oneof payload validation to fail")
	}
}

func TestCashflowAnalysisRequestAndResultValidate(t *testing.T) {
	now := time.Date(2026, 3, 29, 9, 0, 0, 0, time.UTC)
	task := taskspec.DeterministicIntakeService{Now: func() time.Time { return now }}.Parse("请帮我做一份月度财务复盘")
	if task.TaskSpec == nil {
		t.Fatalf("expected monthly review task spec")
	}
	block := planning.ExecutionBlock{
		ID:                "cashflow-review",
		Kind:              planning.ExecutionBlockKindCashflowReview,
		AssignedRecipient: planning.BlockRecipientCashflowAgent,
		Goal:              "cashflow",
		RequiredEvidenceRefs: []planning.ExecutionBlockRequirement{
			{RequirementID: "tx", Type: "transaction_batch", Mandatory: true},
		},
		ExecutionContextView: contextview.ContextViewExecution,
		SuccessCriteria:      []planning.ExecutionBlockSuccessCriteria{{ID: "ok", Description: "ok"}},
		VerificationHints:    []planning.ExecutionBlockVerificationHint{{Rule: "grounding", Description: "grounding"}},
	}
	req := AgentEnvelope{
		Metadata: ProtocolMetadata{
			MessageID:     "msg-cashflow",
			CorrelationID: "corr-cashflow",
			CausationID:   "cause-cashflow",
			EmittedAt:     now,
		},
		Sender:           "workflow",
		Recipient:        "cashflow_agent",
		Task:             *task.TaskSpec,
		StateRef:         StateReference{UserID: "user-1", SnapshotID: "snap-1", Version: 1},
		RequiredEvidence: task.TaskSpec.RequiredEvidence,
		Deadline:         task.TaskSpec.Deadline,
		RiskLevel:        task.TaskSpec.RiskLevel,
		Kind:             MessageKindCashflowAnalysisRequest,
		Payload: AgentRequestBody{
			CashflowAnalysisRequest: &CashflowAnalysisRequestPayload{
				CurrentState: state.FinancialWorldState{
					UserID:  "user-1",
					Version: state.StateVersion{Sequence: 1, SnapshotID: "snap-1", UpdatedAt: now},
				},
				RelevantMemories: []memory.MemoryRecord{
					{ID: "memory-1", Kind: memory.MemoryKindSemantic, Summary: "subscription memory", Source: memory.MemorySource{TaskID: task.TaskSpec.ID}, Confidence: memory.MemoryConfidence{Score: 0.9, Rationale: "direct"}},
				},
				RelevantEvidence: []observation.EvidenceRecord{validEvidence(now, "ev-1", observation.EvidenceTypeTransactionBatch, "transaction_batch")},
				Block:            block,
				ExecutionContext: contextview.BlockExecutionContext{
					View:                contextview.ContextViewExecution,
					PlanID:              "plan-1",
					BlockID:             "cashflow-review",
					BlockKind:           "cashflow_review_block",
					AssignedRecipient:   "cashflow_agent",
					SelectedMemoryIDs:   []string{"memory-1"},
					SelectedEvidenceIDs: []observation.EvidenceID{"ev-1"},
					SelectedStateBlocks: []string{"cashflow_state", "risk_state"},
				},
			},
		},
	}
	if err := req.Validate(); err != nil {
		t.Fatalf("expected valid cashflow analysis request, got %v", err)
	}

	result := AgentResponse{
		Metadata: ProtocolMetadata{
			MessageID:     "msg-cashflow-result",
			CorrelationID: "corr-cashflow",
			CausationID:   "msg-cashflow",
			EmittedAt:     now,
		},
		Sender:           "cashflow_agent",
		Recipient:        "workflow",
		Task:             *task.TaskSpec,
		StateRef:         StateReference{UserID: "user-1", SnapshotID: "snap-1", Version: 1},
		RequiredEvidence: task.TaskSpec.RequiredEvidence,
		Deadline:         task.TaskSpec.Deadline,
		RiskLevel:        task.TaskSpec.RiskLevel,
		Kind:             MessageKindCashflowAnalysisResult,
		Success:          true,
		Body: AgentResultBody{
			CashflowAnalysisResult: &CashflowAnalysisResultPayload{
				Result: analysis.CashflowBlockResult{
					BlockID: "cashflow-review",
					Summary: "cashflow ok",
					DeterministicMetrics: analysis.CashflowDeterministicMetrics{
						MonthlyInflowCents:         100000,
						MonthlyOutflowCents:        50000,
						MonthlyNetIncomeCents:      50000,
						SavingsRate:                0.5,
						DuplicateSubscriptionCount: 1,
						LateNightSpendingFrequency: 0.1,
					},
					EvidenceIDs:   []observation.EvidenceID{"ev-1"},
					MemoryIDsUsed: []string{"memory-1"},
					Confidence:    0.9,
				},
			},
		},
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("expected valid cashflow analysis result, got %v", err)
	}
}

func TestDebtAnalysisRequestRejectsRecipientMismatch(t *testing.T) {
	now := time.Date(2026, 3, 29, 9, 0, 0, 0, time.UTC)
	task := taskspec.DeterministicIntakeService{Now: func() time.Time { return now }}.Parse("提前还贷还是继续投资更合适")
	if task.TaskSpec == nil {
		t.Fatalf("expected debt-vs-invest task spec")
	}
	envelope := AgentEnvelope{
		Metadata: ProtocolMetadata{
			MessageID:     "msg-debt",
			CorrelationID: "corr-debt",
			CausationID:   "cause-debt",
			EmittedAt:     now,
		},
		Sender:           "workflow",
		Recipient:        "debt_agent",
		Task:             *task.TaskSpec,
		StateRef:         StateReference{UserID: "user-1", SnapshotID: "snap-2", Version: 2},
		RequiredEvidence: task.TaskSpec.RequiredEvidence,
		Deadline:         task.TaskSpec.Deadline,
		RiskLevel:        task.TaskSpec.RiskLevel,
		Kind:             MessageKindDebtAnalysisRequest,
		Payload: AgentRequestBody{
			DebtAnalysisRequest: &DebtAnalysisRequestPayload{
				CurrentState: state.FinancialWorldState{
					UserID:  "user-1",
					Version: state.StateVersion{Sequence: 2, SnapshotID: "snap-2", UpdatedAt: now},
				},
				RelevantEvidence: []observation.EvidenceRecord{validEvidence(now, "ev-2", observation.EvidenceTypeDebtObligationSnapshot, "debt_obligation_snapshot")},
				Block: planning.ExecutionBlock{
					ID:                "debt-tradeoff",
					Kind:              planning.ExecutionBlockKindDebtTradeoff,
					AssignedRecipient: planning.BlockRecipientCashflowAgent,
					Goal:              "debt",
					RequiredEvidenceRefs: []planning.ExecutionBlockRequirement{
						{RequirementID: "debt", Type: "debt_obligation_snapshot", Mandatory: true},
					},
					ExecutionContextView: contextview.ContextViewExecution,
					SuccessCriteria:      []planning.ExecutionBlockSuccessCriteria{{ID: "ok", Description: "ok"}},
					VerificationHints:    []planning.ExecutionBlockVerificationHint{{Rule: "grounding", Description: "grounding"}},
				},
				ExecutionContext: contextview.BlockExecutionContext{
					View:                contextview.ContextViewExecution,
					PlanID:              "plan-2",
					BlockID:             "debt-tradeoff",
					BlockKind:           "debt_tradeoff_block",
					AssignedRecipient:   "debt_agent",
					SelectedEvidenceIDs: []observation.EvidenceID{"ev-2"},
				},
			},
		},
	}
	if err := envelope.Validate(); err == nil {
		t.Fatalf("expected debt analysis request to fail on recipient mismatch")
	}
}

func validEvidence(now time.Time, id string, evidenceType observation.EvidenceType, predicate string) observation.EvidenceRecord {
	start := now.Add(-time.Hour)
	return observation.EvidenceRecord{
		ID:   observation.EvidenceID(id),
		Type: evidenceType,
		Source: observation.EvidenceSource{
			Kind:       "fixture",
			Adapter:    "protocol-test",
			Reference:  id,
			Provenance: "protocol fixture",
		},
		TimeRange: observation.EvidenceTimeRange{
			ObservedAt: now,
			Start:      &start,
			End:        &now,
		},
		Confidence: observation.EvidenceConfidence{
			Score:  0.9,
			Reason: "protocol fixture",
		},
		Claims: []observation.EvidenceClaim{
			{Subject: "user-1", Predicate: predicate, Object: "true"},
		},
		Normalization: observation.EvidenceNormalizationResult{
			Status:        observation.EvidenceNormalizationNormalized,
			CanonicalUnit: "unitless",
		},
		Summary:   string(evidenceType),
		CreatedAt: now,
	}
}
