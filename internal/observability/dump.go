package observability

import (
	"encoding/json"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/finance"
	"github.com/kobelakers/personal-cfo-os/internal/model"
	"github.com/kobelakers/personal-cfo-os/internal/prompt"
	"github.com/kobelakers/personal-cfo-os/internal/structured"
	"github.com/kobelakers/personal-cfo-os/internal/verification"
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
		nil,
		nil,
		nil,
		nil,
		nil,
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
	memoryQueries []MemoryQueryTraceRecord,
	memoryRetrievals []MemoryRetrievalTraceRecord,
	memorySelections []MemorySelectionTraceRecord,
	embeddingCalls []EmbeddingCallTraceRecord,
	embeddingUsage []EmbeddingUsageTraceRecord,
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
		MemoryQueries:     memoryQueries,
		MemoryRetrievals:  memoryRetrievals,
		MemorySelections:  memorySelections,
		EmbeddingCalls:    embeddingCalls,
		EmbeddingUsage:    embeddingUsage,
		PolicyDecisions:   policyDecisions,
		PromptRenders:     promptRenders,
		LLMCalls:          llmCalls,
		Usage:             usage,
		StructuredOutputs: structuredOutputs,
		GeneratedAt:       time.Now().UTC(),
	}
}

type TrustTraceBundle struct {
	FinanceMetrics            []finance.MetricRecord
	GroundingVerdicts         []verification.VerificationResult
	NumericValidationVerdicts []verification.VerificationResult
	BusinessRuleVerdicts      []verification.VerificationResult
	PolicyRuleHits            []PolicyRuleHitRecord
	ApprovalTriggers          []ApprovalTriggerRecord
}

func BuildWorkflowTraceDumpWithTrust(
	workflowID string,
	traceID string,
	timeline []TimelineRecord,
	checkpoints []CheckpointRecord,
	agentExecutions []AgentExecutionRecord,
	events []LogEntry,
	memoryAccess []MemoryAccessRecord,
	memoryQueries []MemoryQueryTraceRecord,
	memoryRetrievals []MemoryRetrievalTraceRecord,
	memorySelections []MemorySelectionTraceRecord,
	embeddingCalls []EmbeddingCallTraceRecord,
	embeddingUsage []EmbeddingUsageTraceRecord,
	policyDecisions []PolicyDecisionRecord,
	promptRenders []prompt.PromptRenderTrace,
	llmCalls []model.CallRecord,
	usage []model.UsageRecord,
	structuredOutputs []structured.TraceRecord,
	trust TrustTraceBundle,
) WorkflowTraceDump {
	dump := BuildWorkflowTraceDumpWithIntelligence(
		workflowID,
		traceID,
		timeline,
		checkpoints,
		agentExecutions,
		events,
		memoryAccess,
		memoryQueries,
		memoryRetrievals,
		memorySelections,
		embeddingCalls,
		embeddingUsage,
		policyDecisions,
		promptRenders,
		llmCalls,
		usage,
		structuredOutputs,
	)
	dump.FinanceMetrics = trust.FinanceMetrics
	dump.GroundingVerdicts = trust.GroundingVerdicts
	dump.NumericValidationVerdicts = trust.NumericValidationVerdicts
	dump.BusinessRuleVerdicts = trust.BusinessRuleVerdicts
	dump.PolicyRuleHits = trust.PolicyRuleHits
	dump.ApprovalTriggers = trust.ApprovalTriggers
	return dump
}

func (d WorkflowTraceDump) JSONDump() (string, error) {
	payload, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return "", err
	}
	return string(payload), nil
}
