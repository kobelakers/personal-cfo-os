package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

type OpenAICompatibleConfig struct {
	APIKey         string
	BaseURL        string
	EndpointPath   string
	ReasoningModel string
	FastModel      string
	Transport      Transport
	RetryPolicy    RetryPolicy
	TimeoutPolicy  TimeoutPolicy
	CallRecorder   CallRecorder
	UsageRecorder  UsageRecorder
}

type OpenAICompatibleChatModel struct {
	config OpenAICompatibleConfig
}

func OpenAICompatibleConfigFromEnv() OpenAICompatibleConfig {
	return OpenAICompatibleConfig{
		APIKey:         strings.TrimSpace(os.Getenv("OPENAI_API_KEY")),
		BaseURL:        strings.TrimSpace(os.Getenv("OPENAI_BASE_URL")),
		EndpointPath:   strings.TrimSpace(os.Getenv("OPENAI_CHAT_ENDPOINT_PATH")),
		ReasoningModel: strings.TrimSpace(os.Getenv("OPENAI_REASONING_MODEL")),
		FastModel:      strings.TrimSpace(os.Getenv("OPENAI_FAST_MODEL")),
	}
}

func NewOpenAICompatibleChatModel(config OpenAICompatibleConfig) *OpenAICompatibleChatModel {
	if strings.TrimSpace(config.BaseURL) == "" {
		config.BaseURL = "https://api.openai.com/v1"
	}
	if strings.TrimSpace(config.EndpointPath) == "" {
		config.EndpointPath = "/chat/completions"
	}
	if config.Transport == nil {
		config.Transport = HTTPTransport{Client: &http.Client{}}
	}
	if config.RetryPolicy.MaxAttempts == 0 {
		config.RetryPolicy = DefaultRetryPolicy()
	}
	if config.TimeoutPolicy.RequestTimeout == 0 {
		config.TimeoutPolicy = DefaultTimeoutPolicy()
	}
	return &OpenAICompatibleChatModel{config: config}
}

type openAICompatibleRequest struct {
	Model          string                      `json:"model"`
	Messages       []openAICompatibleMessage   `json:"messages"`
	Temperature    float64                     `json:"temperature,omitempty"`
	MaxTokens      int                         `json:"max_tokens,omitempty"`
	ResponseFormat *openAICompatibleRespFormat `json:"response_format,omitempty"`
}

type openAICompatibleMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAICompatibleRespFormat struct {
	Type string `json:"type"`
}

type openAICompatibleResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		FinishReason string `json:"finish_reason"`
		Message      struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
}

func (m *OpenAICompatibleChatModel) Generate(ctx context.Context, request ModelRequest) (ModelResponse, error) {
	if m == nil {
		return ModelResponse{}, &ProviderError{Category: ProviderErrorConfig, Message: "openai-compatible chat model is nil"}
	}
	modelName, err := resolveModelName(request, m.config.ReasoningModel, m.config.FastModel)
	if err != nil {
		return ModelResponse{}, &ProviderError{Category: ProviderErrorConfig, Message: err.Error()}
	}
	if strings.TrimSpace(m.config.APIKey) == "" {
		return ModelResponse{}, &ProviderError{Category: ProviderErrorAuth, Message: "OPENAI_API_KEY is required for live provider mode"}
	}
	phase := request.GenerationPhase
	if phase == "" {
		phase = GenerationPhaseInitial
	}
	attemptIndex := request.AttemptIndex
	if attemptIndex == 0 {
		attemptIndex = 1
	}
	reqPayload := openAICompatibleRequest{
		Model:       modelName,
		Messages:    make([]openAICompatibleMessage, 0, len(request.Messages)),
		Temperature: request.Temperature,
		MaxTokens:   request.MaxOutputTokens,
	}
	if request.ResponseFormat.Type == ResponseFormatJSONObject {
		reqPayload.ResponseFormat = &openAICompatibleRespFormat{Type: "json_object"}
	}
	for _, message := range request.Messages {
		reqPayload.Messages = append(reqPayload.Messages, openAICompatibleMessage{
			Role:    string(message.Role),
			Content: message.Content,
		})
	}
	body, err := json.Marshal(reqPayload)
	if err != nil {
		return ModelResponse{}, err
	}
	url := strings.TrimRight(m.config.BaseURL, "/") + m.config.EndpointPath
	policy := m.config.RetryPolicy
	if policy.MaxAttempts <= 0 {
		policy = DefaultRetryPolicy()
	}
	var lastErr error
	for attempt := 1; attempt <= policy.MaxAttempts; attempt++ {
		start := time.Now().UTC()
		timeoutCtx, cancel := withRequestTimeout(ctx, m.config.TimeoutPolicy)
		resp, callErr := m.config.Transport.Do(timeoutCtx, TransportRequest{
			Method: "POST",
			URL:    url,
			Headers: map[string]string{
				"Authorization": "Bearer " + m.config.APIKey,
				"Content-Type":  "application/json",
			},
			Body: body,
		})
		cancel()
		record := CallRecord{
			Provider:        "openai-compatible",
			Model:           modelName,
			Profile:         request.Profile,
			WorkflowID:      request.WorkflowID,
			TaskID:          request.TaskID,
			TraceID:         request.TraceID,
			Agent:           request.Agent,
			PromptID:        request.PromptID,
			PromptVersion:   request.PromptVersion,
			GenerationPhase: phase,
			AttemptIndex:    attemptIndex,
			StartedAt:       start,
			CompletedAt:     time.Now().UTC(),
		}
		if callErr != nil {
			providerErr := classifyTransportError(callErr)
			record.ErrorCategory = providerErr.Category
			record.LatencyMS = time.Since(start).Milliseconds()
			if m.config.CallRecorder != nil {
				m.config.CallRecorder.RecordCall(record)
			}
			lastErr = providerErr
			if attempt < policy.MaxAttempts {
				time.Sleep(backoffForAttempt(policy, attempt))
				continue
			}
			return ModelResponse{}, providerErr
		}
		record.LatencyMS = resp.Latency.Milliseconds()
		record.StatusCode = resp.StatusCode
		if resp.StatusCode >= 400 {
			providerErr := coerceOpenAICompatibleError(resp)
			record.ErrorCategory = providerErr.Category
			if m.config.CallRecorder != nil {
				m.config.CallRecorder.RecordCall(record)
			}
			lastErr = providerErr
			if attempt < policy.MaxAttempts && providerErr.Retryable {
				time.Sleep(backoffForAttempt(policy, attempt))
				continue
			}
			return ModelResponse{}, providerErr
		}

		var decoded openAICompatibleResponse
		if err := json.NewDecoder(bytes.NewReader(resp.Body)).Decode(&decoded); err != nil {
			providerErr := &ProviderError{
				Category:  ProviderErrorTransport,
				Message:   fmt.Sprintf("decode provider response: %v", err),
				Retryable: false,
			}
			record.ErrorCategory = providerErr.Category
			if m.config.CallRecorder != nil {
				m.config.CallRecorder.RecordCall(record)
			}
			return ModelResponse{}, providerErr
		}
		if len(decoded.Choices) == 0 {
			providerErr := &ProviderError{
				Category:  ProviderErrorUnavailable,
				Message:   "provider returned no choices",
				Retryable: false,
			}
			record.ErrorCategory = providerErr.Category
			if m.config.CallRecorder != nil {
				m.config.CallRecorder.RecordCall(record)
			}
			return ModelResponse{}, providerErr
		}
		if m.config.CallRecorder != nil {
			m.config.CallRecorder.RecordCall(record)
		}
		usage := UsageStats{
			PromptTokens:     decoded.Usage.PromptTokens,
			CompletionTokens: decoded.Usage.CompletionTokens,
			TotalTokens:      decoded.Usage.TotalTokens,
			EstimatedCostUSD: estimateCostUSD(modelName, request.Profile, decoded.Usage.PromptTokens, decoded.Usage.CompletionTokens),
		}
		if m.config.UsageRecorder != nil {
			m.config.UsageRecorder.RecordUsage(UsageRecord{
				Provider:         "openai-compatible",
				Model:            modelName,
				Profile:          request.Profile,
				WorkflowID:       request.WorkflowID,
				TaskID:           request.TaskID,
				TraceID:          request.TraceID,
				Agent:            request.Agent,
				PromptID:         request.PromptID,
				PromptVersion:    request.PromptVersion,
				GenerationPhase:  phase,
				AttemptIndex:     attemptIndex,
				PromptTokens:     usage.PromptTokens,
				CompletionTokens: usage.CompletionTokens,
				TotalTokens:      usage.TotalTokens,
				EstimatedCostUSD: usage.EstimatedCostUSD,
				RecordedAt:       time.Now().UTC(),
			})
		}
		rawResponse := string(resp.Body)
		return ModelResponse{
			Provider:     "openai-compatible",
			Model:        decoded.Model,
			Profile:      request.Profile,
			ResponseID:   decoded.ID,
			Content:      decoded.Choices[0].Message.Content,
			FinishReason: decoded.Choices[0].FinishReason,
			Usage:        usage,
			Latency:      resp.Latency,
			RawResponse:  rawResponse,
		}, nil
	}
	if lastErr != nil {
		return ModelResponse{}, lastErr
	}
	return ModelResponse{}, &ProviderError{Category: ProviderErrorUnavailable, Message: "provider request failed"}
}

func classifyTransportError(err error) *ProviderError {
	if err == nil {
		return &ProviderError{Category: ProviderErrorTransport, Message: "unknown transport error", Retryable: true}
	}
	providerErr := &ProviderError{
		Category:  ProviderErrorTransport,
		Message:   err.Error(),
		Retryable: true,
	}
	if errors.Is(err, context.DeadlineExceeded) {
		providerErr.Category = ProviderErrorTimeout
		return providerErr
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		providerErr.Category = ProviderErrorTimeout
	}
	return providerErr
}

func coerceOpenAICompatibleError(resp TransportResponse) *ProviderError {
	var decoded openAICompatibleResponse
	if err := json.Unmarshal(resp.Body, &decoded); err != nil {
		message := strings.TrimSpace(string(resp.Body))
		if message == "" {
			message = http.StatusText(resp.StatusCode)
		}
		return providerErrorFromStatus(resp.StatusCode, message)
	}
	if decoded.Error != nil && decoded.Error.Message != "" {
		return providerErrorFromStatus(resp.StatusCode, decoded.Error.Message)
	}
	return providerErrorFromStatus(resp.StatusCode, http.StatusText(resp.StatusCode))
}

func providerErrorFromStatus(statusCode int, message string) *ProviderError {
	err := &ProviderError{
		StatusCode: statusCode,
		Message:    message,
	}
	switch {
	case statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden:
		err.Category = ProviderErrorAuth
	case statusCode == http.StatusTooManyRequests:
		err.Category = ProviderErrorRateLimited
		err.Retryable = true
	case statusCode >= 400 && statusCode < 500:
		err.Category = ProviderErrorHTTP4xx
	case statusCode >= 500:
		err.Category = ProviderErrorHTTP5xx
		err.Retryable = true
	default:
		err.Category = ProviderErrorUnavailable
	}
	return err
}
