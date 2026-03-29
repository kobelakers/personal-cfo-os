package model

import (
	"context"
	"time"
)

type ModelProfile string

const (
	ModelProfilePlannerReasoning ModelProfile = "planner_reasoning"
	ModelProfileCashflowFast     ModelProfile = "cashflow_fast"
)

type GenerationPhase string

const (
	GenerationPhaseInitial GenerationPhase = "initial"
	GenerationPhaseRepair  GenerationPhase = "repair"
)

type MessageRole string

const (
	MessageRoleSystem    MessageRole = "system"
	MessageRoleUser      MessageRole = "user"
	MessageRoleAssistant MessageRole = "assistant"
)

type ResponseFormatType string

const (
	ResponseFormatText       ResponseFormatType = "text"
	ResponseFormatJSONObject ResponseFormatType = "json_object"
)

type ProviderErrorCategory string

const (
	ProviderErrorTimeout     ProviderErrorCategory = "provider_timeout"
	ProviderErrorTransport   ProviderErrorCategory = "provider_transport"
	ProviderErrorHTTP4xx     ProviderErrorCategory = "provider_http_4xx"
	ProviderErrorHTTP5xx     ProviderErrorCategory = "provider_http_5xx"
	ProviderErrorRateLimited ProviderErrorCategory = "provider_rate_limited"
	ProviderErrorAuth        ProviderErrorCategory = "provider_auth"
	ProviderErrorConfig      ProviderErrorCategory = "provider_config"
	ProviderErrorUnavailable ProviderErrorCategory = "provider_unavailable"
	ProviderErrorUnsupported ProviderErrorCategory = "provider_unsupported"
)

type Message struct {
	Role    MessageRole `json:"role"`
	Content string      `json:"content"`
}

type ResponseFormat struct {
	Type ResponseFormatType `json:"type"`
}

type RetryPolicy struct {
	MaxAttempts    int           `json:"max_attempts"`
	InitialBackoff time.Duration `json:"initial_backoff"`
	MaxBackoff     time.Duration `json:"max_backoff"`
}

type TimeoutPolicy struct {
	RequestTimeout time.Duration `json:"request_timeout"`
}

type ModelRequest struct {
	Provider        string          `json:"provider,omitempty"`
	Model           string          `json:"model,omitempty"`
	Profile         ModelProfile    `json:"profile"`
	Messages        []Message       `json:"messages"`
	ResponseFormat  ResponseFormat  `json:"response_format"`
	MaxOutputTokens int             `json:"max_output_tokens,omitempty"`
	Temperature     float64         `json:"temperature,omitempty"`
	WorkflowID      string          `json:"workflow_id,omitempty"`
	TaskID          string          `json:"task_id,omitempty"`
	TraceID         string          `json:"trace_id,omitempty"`
	Agent           string          `json:"agent,omitempty"`
	PromptID        string          `json:"prompt_id,omitempty"`
	PromptVersion   string          `json:"prompt_version,omitempty"`
	GenerationPhase GenerationPhase `json:"generation_phase,omitempty"`
	AttemptIndex    int             `json:"attempt_index,omitempty"`
}

type UsageStats struct {
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	EstimatedCostUSD float64 `json:"estimated_cost_usd"`
}

type ModelResponse struct {
	Provider     string        `json:"provider"`
	Model        string        `json:"model"`
	Profile      ModelProfile  `json:"profile"`
	ResponseID   string        `json:"response_id,omitempty"`
	Content      string        `json:"content"`
	FinishReason string        `json:"finish_reason,omitempty"`
	Usage        UsageStats    `json:"usage"`
	Latency      time.Duration `json:"latency"`
	RawResponse  string        `json:"raw_response,omitempty"`
}

type CallRecord struct {
	Provider        string                `json:"provider"`
	Model           string                `json:"model"`
	Profile         ModelProfile          `json:"profile"`
	WorkflowID      string                `json:"workflow_id,omitempty"`
	TaskID          string                `json:"task_id,omitempty"`
	TraceID         string                `json:"trace_id,omitempty"`
	Agent           string                `json:"agent,omitempty"`
	PromptID        string                `json:"prompt_id,omitempty"`
	PromptVersion   string                `json:"prompt_version,omitempty"`
	GenerationPhase GenerationPhase       `json:"generation_phase,omitempty"`
	AttemptIndex    int                   `json:"attempt_index,omitempty"`
	LatencyMS       int64                 `json:"latency_ms"`
	ErrorCategory   ProviderErrorCategory `json:"error_category,omitempty"`
	StatusCode      int                   `json:"status_code,omitempty"`
	StartedAt       time.Time             `json:"started_at"`
	CompletedAt     time.Time             `json:"completed_at"`
}

type UsageRecord struct {
	Provider         string          `json:"provider"`
	Model            string          `json:"model"`
	Profile          ModelProfile    `json:"profile"`
	WorkflowID       string          `json:"workflow_id,omitempty"`
	TaskID           string          `json:"task_id,omitempty"`
	TraceID          string          `json:"trace_id,omitempty"`
	Agent            string          `json:"agent,omitempty"`
	PromptID         string          `json:"prompt_id,omitempty"`
	PromptVersion    string          `json:"prompt_version,omitempty"`
	GenerationPhase  GenerationPhase `json:"generation_phase,omitempty"`
	AttemptIndex     int             `json:"attempt_index,omitempty"`
	PromptTokens     int             `json:"prompt_tokens"`
	CompletionTokens int             `json:"completion_tokens"`
	TotalTokens      int             `json:"total_tokens"`
	EstimatedCostUSD float64         `json:"estimated_cost_usd"`
	RecordedAt       time.Time       `json:"recorded_at"`
}

type CallRecorder interface {
	RecordCall(record CallRecord)
}

type UsageRecorder interface {
	RecordUsage(record UsageRecord)
}

type ChatModel interface {
	Generate(ctx context.Context, request ModelRequest) (ModelResponse, error)
}

type StructuredGenerationRequest struct {
	ModelRequest ModelRequest `json:"model_request"`
}

type StructuredGenerationResult struct {
	Response ModelResponse `json:"response"`
	Content  string        `json:"content"`
}

type StructuredGenerationFailure struct {
	Category ProviderErrorCategory `json:"category"`
	Message  string                `json:"message"`
}

type StructuredGenerator interface {
	Generate(ctx context.Context, request StructuredGenerationRequest) (StructuredGenerationResult, error)
}

type ProviderError struct {
	Category   ProviderErrorCategory
	StatusCode int
	Message    string
	Retryable  bool
}

func (e *ProviderError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}
