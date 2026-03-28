package protocol

import (
	"encoding/json"
	"testing"
	"time"

	contextview "github.com/kobelakers/personal-cfo-os/internal/context"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
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
