package protocol

import (
	"encoding/json"
	"testing"
	"time"

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
