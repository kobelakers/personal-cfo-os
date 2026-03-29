package observability_test

import (
	"testing"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/governance"
	"github.com/kobelakers/personal-cfo-os/internal/memory"
	"github.com/kobelakers/personal-cfo-os/internal/observability"
	"github.com/kobelakers/personal-cfo-os/internal/runtime"
)

func TestWorkflowTraceDumpBuildsUnifiedStructuredOutput(t *testing.T) {
	now := time.Date(2026, 3, 28, 19, 0, 0, 0, time.UTC)
	timeline := runtime.WorkflowTimeline{
		WorkflowID: "workflow-1",
		TraceID:    "trace-1",
		Entries: []runtime.WorkflowTimelineEntry{
			{State: runtime.WorkflowStatePlanning, Event: "checkpoint_created", Summary: "observation completed", OccurredAt: now},
		},
	}
	journal := runtime.CheckpointJournal{
		Checkpoints: []runtime.CheckpointRecord{
			{ID: "cp-1", WorkflowID: "workflow-1", State: runtime.WorkflowStatePlanning, ResumeState: runtime.WorkflowStateActing, StateVersion: 2, Summary: "before act", CapturedAt: now},
		},
	}
	log := observability.EventLog{}
	log.Append(observability.LogEntry{TraceID: "trace-1", CorrelationID: "trace-1", Category: "checkpoint", Message: "before act", OccurredAt: now})
	agentTrace := &observability.AgentTraceLog{}
	agentTrace.Append(observability.AgentExecutionRecord{
		DispatchID:          "dispatch-1",
		TraceID:             "trace-1",
		Recipient:           "cashflow_agent",
		RequestKind:         "cashflow_analysis_request",
		ResultKind:          "cashflow_analysis_result",
		PlanID:              "plan-1",
		PlanBlockIDs:        []string{"cashflow-review", "debt-review"},
		BlockID:             "cashflow-review",
		BlockKind:           "cashflow_review_block",
		SelectedMemoryIDs:   []string{"memory-1"},
		SelectedEvidenceIDs: []string{"ev-1"},
		SelectedStateBlocks: []string{"cashflow_state", "risk_state"},
		Lifecycle:           observability.AgentLifecycleCompleted,
		CorrelationID:       "trace-1",
		CausationID:         "msg-1",
		RequestMessageID:    "msg-1",
		ResultMessageID:     "msg-2",
		WorkflowEventTypes:  []string{"verification_failed"},
		ResultSummary:       "现金流块结论：本月净结余稳定。",
		OccurredAt:          now,
	})
	memoryRecords := memory.ToObservabilityRecords([]memory.MemoryAccessAudit{
		{MemoryID: "memory-1", Accessor: "hybrid_retriever", Purpose: "monthly review", Action: "retrieve", AccessedAt: now},
	})
	policyRecords := governance.ToObservabilityRecords([]governance.AuditEvent{
		{ID: "audit-1", Actor: "governance_agent", Action: "approval", Resource: "task-1", Outcome: "require_approval", Reason: "high risk", OccurredAt: now, CorrelationID: "trace-1"},
	})

	dump := observability.BuildWorkflowTraceDump("workflow-1", "trace-1", timeline.Records(), journal.Records(), agentTrace.Records(), log.Entries(), memoryRecords, policyRecords)
	payload, err := dump.JSONDump()
	if err != nil {
		t.Fatalf("json dump trace: %v", err)
	}
	if dump.WorkflowID != "workflow-1" || len(dump.Timeline) != 1 || len(dump.Checkpoints) != 1 || len(dump.AgentExecutions) != 1 || len(dump.MemoryAccess) != 1 || len(dump.PolicyDecisions) != 1 {
		t.Fatalf("unexpected dump shape: %+v", dump)
	}
	if dump.AgentExecutions[0].PlanID != "plan-1" || dump.AgentExecutions[0].BlockID != "cashflow-review" || len(dump.AgentExecutions[0].SelectedEvidenceIDs) != 1 {
		t.Fatalf("expected dump to preserve block-level agent execution details, got %+v", dump.AgentExecutions[0])
	}
	replay := observability.NewReplayBundle("monthly-review", dump, map[string]string{"phase": "phase2"})
	if replay.Scenario == "" || payload == "" || len(replay.Trace.AgentExecutions) != 1 {
		t.Fatalf("expected replay bundle and dump payload")
	}
}
