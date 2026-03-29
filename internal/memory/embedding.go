package memory

import (
	"context"
	"time"
)

type EmbeddingErrorCategory string

const (
	EmbeddingErrorTimeout     EmbeddingErrorCategory = "embedding_timeout"
	EmbeddingErrorTransport   EmbeddingErrorCategory = "embedding_transport"
	EmbeddingErrorHTTP4xx     EmbeddingErrorCategory = "embedding_http_4xx"
	EmbeddingErrorHTTP5xx     EmbeddingErrorCategory = "embedding_http_5xx"
	EmbeddingErrorRateLimited EmbeddingErrorCategory = "embedding_rate_limited"
	EmbeddingErrorAuth        EmbeddingErrorCategory = "embedding_auth"
	EmbeddingErrorConfig      EmbeddingErrorCategory = "embedding_config"
	EmbeddingErrorUnsupported EmbeddingErrorCategory = "embedding_unsupported"
)

type EmbeddingRequest struct {
	Provider   string `json:"provider,omitempty"`
	Model      string `json:"model,omitempty"`
	Input      string `json:"input"`
	WorkflowID string `json:"workflow_id,omitempty"`
	TaskID     string `json:"task_id,omitempty"`
	TraceID    string `json:"trace_id,omitempty"`
	Actor      string `json:"actor,omitempty"`
	QueryID    string `json:"query_id,omitempty"`
}

type EmbeddingUsageRecord struct {
	Provider         string    `json:"provider"`
	Model            string    `json:"model"`
	WorkflowID       string    `json:"workflow_id,omitempty"`
	TaskID           string    `json:"task_id,omitempty"`
	TraceID          string    `json:"trace_id,omitempty"`
	Actor            string    `json:"actor,omitempty"`
	QueryID          string    `json:"query_id,omitempty"`
	InputTokens      int       `json:"input_tokens"`
	EstimatedCostUSD float64   `json:"estimated_cost_usd"`
	RecordedAt       time.Time `json:"recorded_at"`
}

type EmbeddingCallRecord struct {
	Provider      string                 `json:"provider"`
	Model         string                 `json:"model"`
	WorkflowID    string                 `json:"workflow_id,omitempty"`
	TaskID        string                 `json:"task_id,omitempty"`
	TraceID       string                 `json:"trace_id,omitempty"`
	Actor         string                 `json:"actor,omitempty"`
	QueryID       string                 `json:"query_id,omitempty"`
	LatencyMS     int64                  `json:"latency_ms"`
	ErrorCategory EmbeddingErrorCategory `json:"error_category,omitempty"`
	StatusCode    int                    `json:"status_code,omitempty"`
	StartedAt     time.Time              `json:"started_at"`
	CompletedAt   time.Time              `json:"completed_at"`
}

type EmbeddingResponse struct {
	Provider    string               `json:"provider"`
	Model       string               `json:"model"`
	Vector      []float64            `json:"vector"`
	Dimensions  int                  `json:"dimensions"`
	ResponseID  string               `json:"response_id,omitempty"`
	Latency     time.Duration        `json:"latency"`
	Usage       EmbeddingUsageRecord `json:"usage"`
	RawResponse string               `json:"raw_response,omitempty"`
}

type EmbeddingCallRecorder interface {
	RecordEmbeddingCall(record EmbeddingCallRecord)
}

type EmbeddingUsageRecorder interface {
	RecordEmbeddingUsage(record EmbeddingUsageRecord)
}

type EmbeddingProviderError struct {
	Category   EmbeddingErrorCategory
	StatusCode int
	Message    string
	Retryable  bool
}

func (e *EmbeddingProviderError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

type embeddingContextKey string

const embeddingRequestContextKey embeddingContextKey = "memory.embedding.request"

func contextWithEmbeddingRequest(ctx context.Context, request EmbeddingRequest) context.Context {
	return context.WithValue(ctx, embeddingRequestContextKey, request)
}

func embeddingRequestFromContext(ctx context.Context) (EmbeddingRequest, bool) {
	request, ok := ctx.Value(embeddingRequestContextKey).(EmbeddingRequest)
	return request, ok
}

