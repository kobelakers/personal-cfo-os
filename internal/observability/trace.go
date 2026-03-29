package observability

import (
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/model"
	"github.com/kobelakers/personal-cfo-os/internal/prompt"
	"github.com/kobelakers/personal-cfo-os/internal/structured"
)

type TimelineRecord struct {
	State      string    `json:"state"`
	Event      string    `json:"event"`
	Summary    string    `json:"summary"`
	OccurredAt time.Time `json:"occurred_at"`
}

type CheckpointRecord struct {
	ID           string    `json:"id"`
	WorkflowID   string    `json:"workflow_id"`
	State        string    `json:"state"`
	ResumeState  string    `json:"resume_state"`
	StateVersion uint64    `json:"state_version"`
	Summary      string    `json:"summary"`
	CapturedAt   time.Time `json:"captured_at"`
}

type PolicyDecisionRecord struct {
	ID            string    `json:"id"`
	Actor         string    `json:"actor"`
	Action        string    `json:"action"`
	Resource      string    `json:"resource"`
	Outcome       string    `json:"outcome"`
	Reason        string    `json:"reason"`
	OccurredAt    time.Time `json:"occurred_at"`
	CorrelationID string    `json:"correlation_id"`
}

type WorkflowTraceDump struct {
	WorkflowID        string                     `json:"workflow_id"`
	TraceID           string                     `json:"trace_id"`
	Timeline          []TimelineRecord           `json:"timeline,omitempty"`
	Checkpoints       []CheckpointRecord         `json:"checkpoints,omitempty"`
	AgentExecutions   []AgentExecutionRecord     `json:"agent_executions,omitempty"`
	Events            []LogEntry                 `json:"events,omitempty"`
	MemoryAccess      []MemoryAccessRecord       `json:"memory_access,omitempty"`
	MemoryQueries     []MemoryQueryTraceRecord   `json:"memory_queries,omitempty"`
	MemoryRetrievals  []MemoryRetrievalTraceRecord `json:"memory_retrievals,omitempty"`
	MemorySelections  []MemorySelectionTraceRecord `json:"memory_selections,omitempty"`
	EmbeddingCalls    []EmbeddingCallTraceRecord `json:"embedding_calls,omitempty"`
	EmbeddingUsage    []EmbeddingUsageTraceRecord `json:"embedding_usage,omitempty"`
	PolicyDecisions   []PolicyDecisionRecord     `json:"policy_decisions,omitempty"`
	PromptRenders     []prompt.PromptRenderTrace `json:"prompt_renders,omitempty"`
	LLMCalls          []model.CallRecord         `json:"llm_calls,omitempty"`
	Usage             []model.UsageRecord        `json:"usage,omitempty"`
	StructuredOutputs []structured.TraceRecord   `json:"structured_outputs,omitempty"`
	GeneratedAt       time.Time                  `json:"generated_at"`
}
