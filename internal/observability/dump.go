package observability

import (
	"encoding/json"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/model"
	"github.com/kobelakers/personal-cfo-os/internal/prompt"
	"github.com/kobelakers/personal-cfo-os/internal/structured"
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
	return BuildWorkflowTraceDumpWithIntelligence(
		workflowID,
		traceID,
		timeline,
		checkpoints,
		agentExecutions,
		events,
		memoryAccess,
		policyDecisions,
		nil,
		nil,
		nil,
		nil,
	)
}

func BuildWorkflowTraceDumpWithIntelligence(
	workflowID string,
	traceID string,
	timeline []TimelineRecord,
	checkpoints []CheckpointRecord,
	agentExecutions []AgentExecutionRecord,
	events []LogEntry,
	memoryAccess []MemoryAccessRecord,
	policyDecisions []PolicyDecisionRecord,
	promptRenders []prompt.PromptRenderTrace,
	llmCalls []model.CallRecord,
	usage []model.UsageRecord,
	structuredOutputs []structured.TraceRecord,
) WorkflowTraceDump {
	return WorkflowTraceDump{
		WorkflowID:        workflowID,
		TraceID:           traceID,
		Timeline:          timeline,
		Checkpoints:       checkpoints,
		AgentExecutions:   agentExecutions,
		Events:            events,
		MemoryAccess:      memoryAccess,
		PolicyDecisions:   policyDecisions,
		PromptRenders:     promptRenders,
		LLMCalls:          llmCalls,
		Usage:             usage,
		StructuredOutputs: structuredOutputs,
		GeneratedAt:       time.Now().UTC(),
	}
}

func (d WorkflowTraceDump) JSONDump() (string, error) {
	payload, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return "", err
	}
	return string(payload), nil
}
