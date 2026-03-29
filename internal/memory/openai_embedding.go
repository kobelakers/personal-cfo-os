package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/model"
)

type OpenAIEmbeddingConfig struct {
	APIKey        string
	BaseURL       string
	EndpointPath  string
	EmbeddingModel string
	Transport     model.Transport
	RetryPolicy   model.RetryPolicy
	TimeoutPolicy model.TimeoutPolicy
	CallRecorder  EmbeddingCallRecorder
	UsageRecorder EmbeddingUsageRecorder
}

type OpenAIEmbeddingProvider struct {
	config OpenAIEmbeddingConfig
}

func OpenAIEmbeddingConfigFromEnv() OpenAIEmbeddingConfig {
	return OpenAIEmbeddingConfig{
		APIKey:         strings.TrimSpace(os.Getenv("OPENAI_API_KEY")),
		BaseURL:        strings.TrimSpace(os.Getenv("OPENAI_BASE_URL")),
		EndpointPath:   strings.TrimSpace(os.Getenv("OPENAI_EMBEDDING_ENDPOINT_PATH")),
		EmbeddingModel: strings.TrimSpace(os.Getenv("OPENAI_EMBEDDING_MODEL")),
	}
}

func NewOpenAIEmbeddingProvider(config OpenAIEmbeddingConfig) *OpenAIEmbeddingProvider {
	if strings.TrimSpace(config.BaseURL) == "" {
		config.BaseURL = "https://api.openai.com/v1"
	}
	if strings.TrimSpace(config.EndpointPath) == "" {
		config.EndpointPath = "/embeddings"
	}
	if config.Transport == nil {
		config.Transport = model.HTTPTransport{}
	}
	if config.RetryPolicy.MaxAttempts == 0 {
		config.RetryPolicy = model.DefaultRetryPolicy()
	}
	if config.TimeoutPolicy.RequestTimeout == 0 {
		config.TimeoutPolicy = model.DefaultTimeoutPolicy()
	}
	return &OpenAIEmbeddingProvider{config: config}
}

func (p *OpenAIEmbeddingProvider) Embed(ctx context.Context, text string) ([]float64, error) {
	request := EmbeddingRequest{Input: text}
	if fromCtx, ok := embeddingRequestFromContext(ctx); ok {
		request = fromCtx
		request.Input = text
	}
	response, err := p.GenerateEmbedding(ctx, request)
	if err != nil {
		return nil, err
	}
	return response.Vector, nil
}

func (p *OpenAIEmbeddingProvider) GenerateEmbedding(ctx context.Context, request EmbeddingRequest) (EmbeddingResponse, error) {
	if p == nil {
		return EmbeddingResponse{}, &EmbeddingProviderError{Category: EmbeddingErrorConfig, Message: "openai embedding provider is nil"}
	}
	modelName := strings.TrimSpace(request.Model)
	if modelName == "" {
		modelName = strings.TrimSpace(p.config.EmbeddingModel)
	}
	if modelName == "" {
		return EmbeddingResponse{}, &EmbeddingProviderError{Category: EmbeddingErrorConfig, Message: "OPENAI_EMBEDDING_MODEL is required for live embedding mode"}
	}
	if strings.TrimSpace(p.config.APIKey) == "" {
		return EmbeddingResponse{}, &EmbeddingProviderError{Category: EmbeddingErrorAuth, Message: "OPENAI_API_KEY is required for live embedding mode"}
	}
	payload, err := json.Marshal(map[string]any{
		"model": modelName,
		"input": request.Input,
	})
	if err != nil {
		return EmbeddingResponse{}, err
	}
	url := strings.TrimRight(p.config.BaseURL, "/") + p.config.EndpointPath
	var lastErr error
	for attempt := 1; attempt <= p.config.RetryPolicy.MaxAttempts; attempt++ {
		start := time.Now().UTC()
		timeout := p.config.TimeoutPolicy.RequestTimeout
		if timeout <= 0 {
			timeout = model.DefaultTimeoutPolicy().RequestTimeout
		}
		timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
		resp, callErr := p.config.Transport.Do(timeoutCtx, model.TransportRequest{
			Method: "POST",
			URL:    url,
			Headers: map[string]string{
				"Authorization": "Bearer " + p.config.APIKey,
				"Content-Type":  "application/json",
			},
			Body: payload,
		})
		cancel()
		callRecord := EmbeddingCallRecord{
			Provider:   "openai-compatible-embeddings",
			Model:      modelName,
			WorkflowID: request.WorkflowID,
			TaskID:     request.TaskID,
			TraceID:    request.TraceID,
			Actor:      request.Actor,
			QueryID:    request.QueryID,
			StartedAt:  start,
			CompletedAt: time.Now().UTC(),
		}
		if callErr != nil {
			providerErr := classifyEmbeddingTransportError(callErr)
			callRecord.ErrorCategory = providerErr.Category
			callRecord.LatencyMS = time.Since(start).Milliseconds()
			if p.config.CallRecorder != nil {
				p.config.CallRecorder.RecordEmbeddingCall(callRecord)
			}
			lastErr = providerErr
			if attempt < p.config.RetryPolicy.MaxAttempts && providerErr.Retryable {
				time.Sleep(embeddingBackoffForAttempt(p.config.RetryPolicy, attempt))
				continue
			}
			return EmbeddingResponse{}, providerErr
		}
		callRecord.StatusCode = resp.StatusCode
		callRecord.LatencyMS = resp.Latency.Milliseconds()
		if resp.StatusCode >= 400 {
			providerErr := coerceEmbeddingError(resp.StatusCode, resp.Body)
			callRecord.ErrorCategory = providerErr.Category
			if p.config.CallRecorder != nil {
				p.config.CallRecorder.RecordEmbeddingCall(callRecord)
			}
			lastErr = providerErr
			if attempt < p.config.RetryPolicy.MaxAttempts && providerErr.Retryable {
				time.Sleep(embeddingBackoffForAttempt(p.config.RetryPolicy, attempt))
				continue
			}
			return EmbeddingResponse{}, providerErr
		}
		var decoded struct {
			Data []struct {
				Embedding []float64 `json:"embedding"`
			} `json:"data"`
			Usage struct {
				PromptTokens int `json:"prompt_tokens"`
				TotalTokens  int `json:"total_tokens"`
			} `json:"usage"`
		}
		if err := json.NewDecoder(bytes.NewReader(resp.Body)).Decode(&decoded); err != nil {
			return EmbeddingResponse{}, &EmbeddingProviderError{Category: EmbeddingErrorTransport, Message: fmt.Sprintf("decode embedding response: %v", err)}
		}
		if len(decoded.Data) == 0 {
			return EmbeddingResponse{}, &EmbeddingProviderError{Category: EmbeddingErrorUnsupported, Message: "embedding provider returned no vectors"}
		}
		if p.config.CallRecorder != nil {
			p.config.CallRecorder.RecordEmbeddingCall(callRecord)
		}
		usage := EmbeddingUsageRecord{
			Provider:         "openai-compatible-embeddings",
			Model:            modelName,
			WorkflowID:       request.WorkflowID,
			TaskID:           request.TaskID,
			TraceID:          request.TraceID,
			Actor:            request.Actor,
			QueryID:          request.QueryID,
			InputTokens:      maxInt(decoded.Usage.TotalTokens, decoded.Usage.PromptTokens),
			EstimatedCostUSD: float64(maxInt(decoded.Usage.TotalTokens, decoded.Usage.PromptTokens)) / 1_000_000.0,
			RecordedAt:       time.Now().UTC(),
		}
		if p.config.UsageRecorder != nil {
			p.config.UsageRecorder.RecordEmbeddingUsage(usage)
		}
		return EmbeddingResponse{
			Provider:    "openai-compatible-embeddings",
			Model:       modelName,
			Vector:      normalizeVector(decoded.Data[0].Embedding),
			Dimensions:  len(decoded.Data[0].Embedding),
			Latency:     resp.Latency,
			Usage:       usage,
			RawResponse: string(resp.Body),
		}, nil
	}
	if lastErr != nil {
		return EmbeddingResponse{}, lastErr
	}
	return EmbeddingResponse{}, &EmbeddingProviderError{Category: EmbeddingErrorUnsupported, Message: "embedding provider failed without classified error"}
}

func embeddingBackoffForAttempt(policy model.RetryPolicy, attempt int) time.Duration {
	backoff := policy.InitialBackoff
	if backoff <= 0 {
		backoff = 200 * time.Millisecond
	}
	for i := 1; i < attempt; i++ {
		backoff *= 2
		if policy.MaxBackoff > 0 && backoff > policy.MaxBackoff {
			return policy.MaxBackoff
		}
	}
	if policy.MaxBackoff > 0 && backoff > policy.MaxBackoff {
		return policy.MaxBackoff
	}
	return backoff
}

func classifyEmbeddingTransportError(err error) *EmbeddingProviderError {
	var netErr net.Error
	switch {
	case errors.As(err, &netErr) && netErr.Timeout():
		return &EmbeddingProviderError{Category: EmbeddingErrorTimeout, Message: err.Error(), Retryable: true}
	default:
		return &EmbeddingProviderError{Category: EmbeddingErrorTransport, Message: err.Error(), Retryable: true}
	}
}

func coerceEmbeddingError(status int, body []byte) *EmbeddingProviderError {
	category := EmbeddingErrorHTTP4xx
	retryable := false
	switch {
	case status == 401 || status == 403:
		category = EmbeddingErrorAuth
	case status == 429:
		category = EmbeddingErrorRateLimited
		retryable = true
	case status >= 500:
		category = EmbeddingErrorHTTP5xx
		retryable = true
	}
	return &EmbeddingProviderError{
		Category:   category,
		StatusCode: status,
		Message:    strings.TrimSpace(string(body)),
		Retryable:  retryable,
	}
}
