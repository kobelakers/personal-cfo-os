package observability

import (
	"encoding/json"
	"time"
)

func BuildWorkflowTraceDump(
	workflowID string,
	traceID string,
	timeline []TimelineRecord,
	checkpoints []CheckpointRecord,
	agentExecutions []AgentExecutionRecord,
	events []LogEntry,
	memoryAccess []MemoryAccessRecord,
	policyDecisions []PolicyDecisionRecord,
) WorkflowTraceDump {
	return WorkflowTraceDump{
		WorkflowID:      workflowID,
		TraceID:         traceID,
		Timeline:        timeline,
		Checkpoints:     checkpoints,
		AgentExecutions: agentExecutions,
		Events:          events,
		MemoryAccess:    memoryAccess,
		PolicyDecisions: policyDecisions,
		GeneratedAt:     time.Now().UTC(),
	}
}

func (d WorkflowTraceDump) JSONDump() (string, error) {
	payload, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return "", err
	}
	return string(payload), nil
}
