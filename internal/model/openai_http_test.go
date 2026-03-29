package model

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestOpenAICompatibleChatModelGenerateHappyPath(t *testing.T) {
	var (
		mu        sync.Mutex
		callCount int
		callLog   []CallRecord
		usageLog  []UsageRecord
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		callCount++
		mu.Unlock()
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("expected bearer auth header, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"resp-1",
			"model":"reasoning-live",
			"choices":[{"finish_reason":"stop","message":{"content":"{\"ok\":true}"}}],
			"usage":{"prompt_tokens":120,"completion_tokens":80,"total_tokens":200}
		}`))
	}))
	defer server.Close()

	model := NewOpenAICompatibleChatModel(OpenAICompatibleConfig{
		APIKey:         "test-key",
		BaseURL:        server.URL,
		EndpointPath:   "",
		ReasoningModel: "reasoning-live",
		FastModel:      "fast-live",
		Transport:      HTTPTransport{Client: server.Client()},
		RetryPolicy:    RetryPolicy{MaxAttempts: 1, InitialBackoff: time.Millisecond, MaxBackoff: time.Millisecond},
		TimeoutPolicy:  TimeoutPolicy{RequestTimeout: time.Second},
		CallRecorder:   callRecorderFunc(func(record CallRecord) { callLog = append(callLog, record) }),
		UsageRecorder:  usageRecorderFunc(func(record UsageRecord) { usageLog = append(usageLog, record) }),
	})

	response, err := model.Generate(context.Background(), ModelRequest{
		Profile:         ModelProfilePlannerReasoning,
		Messages:        []Message{{Role: MessageRoleSystem, Content: "system"}, {Role: MessageRoleUser, Content: "user"}},
		ResponseFormat:  ResponseFormat{Type: ResponseFormatJSONObject},
		MaxOutputTokens: 256,
		Temperature:     0.1,
		WorkflowID:      "workflow-1",
		TaskID:          "task-1",
		TraceID:         "trace-1",
		Agent:           "planner_agent",
		PromptID:        "planner.monthly_review.v1",
		PromptVersion:   "v1",
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if response.Provider != "openai-compatible" || response.Content != "{\"ok\":true}" {
		t.Fatalf("unexpected response: %+v", response)
	}
	if response.Usage.TotalTokens != 200 {
		t.Fatalf("expected total tokens 200, got %+v", response.Usage)
	}
	if callCount != 1 {
		t.Fatalf("expected one provider call, got %d", callCount)
	}
	if len(callLog) != 1 || callLog[0].PromptID != "planner.monthly_review.v1" {
		t.Fatalf("expected call trace with prompt metadata, got %+v", callLog)
	}
	if len(usageLog) != 1 || usageLog[0].TotalTokens != 200 {
		t.Fatalf("expected usage trace, got %+v", usageLog)
	}
}

func TestOpenAICompatibleChatModelRetriesRateLimitedRequests(t *testing.T) {
	var callCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"message":"rate limited"}}`))
			return
		}
		_, _ = w.Write([]byte(`{
			"id":"resp-2",
			"model":"fast-live",
			"choices":[{"finish_reason":"stop","message":{"content":"{\"ok\":true}"}}],
			"usage":{"prompt_tokens":20,"completion_tokens":10,"total_tokens":30}
		}`))
	}))
	defer server.Close()

	model := NewOpenAICompatibleChatModel(OpenAICompatibleConfig{
		APIKey:         "test-key",
		BaseURL:        server.URL,
		ReasoningModel: "reasoning-live",
		FastModel:      "fast-live",
		Transport:      HTTPTransport{Client: server.Client()},
		RetryPolicy:    RetryPolicy{MaxAttempts: 2, InitialBackoff: time.Millisecond, MaxBackoff: time.Millisecond},
		TimeoutPolicy:  TimeoutPolicy{RequestTimeout: time.Second},
	})

	response, err := model.Generate(context.Background(), ModelRequest{
		Profile:        ModelProfileCashflowFast,
		Messages:       []Message{{Role: MessageRoleUser, Content: "user"}},
		ResponseFormat: ResponseFormat{Type: ResponseFormatJSONObject},
	})
	if err != nil {
		t.Fatalf("generate with retry: %v", err)
	}
	if callCount != 2 {
		t.Fatalf("expected retry to issue two calls, got %d", callCount)
	}
	if response.Content != "{\"ok\":true}" {
		t.Fatalf("unexpected retry response: %+v", response)
	}
}

func TestOpenAICompatibleChatModelClassifiesTimeouts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"late","choices":[{"message":{"content":"{}"}}]}`))
	}))
	defer server.Close()

	model := NewOpenAICompatibleChatModel(OpenAICompatibleConfig{
		APIKey:         "test-key",
		BaseURL:        server.URL,
		ReasoningModel: "reasoning-live",
		FastModel:      "fast-live",
		Transport:      HTTPTransport{Client: server.Client()},
		RetryPolicy:    RetryPolicy{MaxAttempts: 1, InitialBackoff: time.Millisecond, MaxBackoff: time.Millisecond},
		TimeoutPolicy:  TimeoutPolicy{RequestTimeout: 5 * time.Millisecond},
	})

	_, err := model.Generate(context.Background(), ModelRequest{
		Profile:        ModelProfilePlannerReasoning,
		Messages:       []Message{{Role: MessageRoleUser, Content: "timeout"}},
		ResponseFormat: ResponseFormat{Type: ResponseFormatJSONObject},
	})
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("expected provider error, got %v", err)
	}
	if providerErr.Category != ProviderErrorTimeout {
		t.Fatalf("expected timeout category, got %+v", providerErr)
	}
}

func TestOpenAICompatibleChatModelClassifiesHTTP5xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":{"message":"upstream unavailable"}}`))
	}))
	defer server.Close()

	model := NewOpenAICompatibleChatModel(OpenAICompatibleConfig{
		APIKey:         "test-key",
		BaseURL:        server.URL,
		ReasoningModel: "reasoning-live",
		FastModel:      "fast-live",
		Transport:      HTTPTransport{Client: server.Client()},
		RetryPolicy:    RetryPolicy{MaxAttempts: 1, InitialBackoff: time.Millisecond, MaxBackoff: time.Millisecond},
		TimeoutPolicy:  TimeoutPolicy{RequestTimeout: time.Second},
	})

	_, err := model.Generate(context.Background(), ModelRequest{
		Profile:        ModelProfilePlannerReasoning,
		Messages:       []Message{{Role: MessageRoleUser, Content: "fail"}},
		ResponseFormat: ResponseFormat{Type: ResponseFormatJSONObject},
	})
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("expected provider error, got %v", err)
	}
	if providerErr.Category != ProviderErrorHTTP5xx {
		t.Fatalf("expected http 5xx category, got %+v", providerErr)
	}
}

func TestOpenAICompatibleChatModelHandlesMalformedBodies(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"oops"`))
	}))
	defer server.Close()

	model := NewOpenAICompatibleChatModel(OpenAICompatibleConfig{
		APIKey:         "test-key",
		BaseURL:        server.URL,
		ReasoningModel: "reasoning-live",
		FastModel:      "fast-live",
		Transport:      HTTPTransport{Client: server.Client()},
		RetryPolicy:    RetryPolicy{MaxAttempts: 1, InitialBackoff: time.Millisecond, MaxBackoff: time.Millisecond},
		TimeoutPolicy:  TimeoutPolicy{RequestTimeout: time.Second},
	})

	_, err := model.Generate(context.Background(), ModelRequest{
		Profile:        ModelProfileCashflowFast,
		Messages:       []Message{{Role: MessageRoleUser, Content: "bad-json"}},
		ResponseFormat: ResponseFormat{Type: ResponseFormatJSONObject},
	})
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("expected provider error, got %v", err)
	}
	if providerErr.Category != ProviderErrorTransport {
		t.Fatalf("expected malformed body to surface as transport error, got %+v", providerErr)
	}
}

func TestOpenAICompatibleChatModelRequiresReasoningModelInLiveMode(t *testing.T) {
	model := NewOpenAICompatibleChatModel(OpenAICompatibleConfig{
		APIKey: "test-key",
	})

	_, err := model.Generate(context.Background(), ModelRequest{
		Profile:        ModelProfilePlannerReasoning,
		Messages:       []Message{{Role: MessageRoleUser, Content: "plan"}},
		ResponseFormat: ResponseFormat{Type: ResponseFormatJSONObject},
	})
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("expected provider config error, got %v", err)
	}
	if providerErr.Category != ProviderErrorConfig || providerErr.Message == "" {
		t.Fatalf("expected config error, got %+v", providerErr)
	}
}

func TestOpenAICompatibleChatModelRequiresFastModelInLiveMode(t *testing.T) {
	model := NewOpenAICompatibleChatModel(OpenAICompatibleConfig{
		APIKey:         "test-key",
		ReasoningModel: "reasoning-live",
	})

	_, err := model.Generate(context.Background(), ModelRequest{
		Profile:        ModelProfileCashflowFast,
		Messages:       []Message{{Role: MessageRoleUser, Content: "analyze"}},
		ResponseFormat: ResponseFormat{Type: ResponseFormatJSONObject},
	})
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("expected provider config error, got %v", err)
	}
	if providerErr.Category != ProviderErrorConfig || providerErr.Message == "" {
		t.Fatalf("expected config error, got %+v", providerErr)
	}
}

type callRecorderFunc func(record CallRecord)

func (f callRecorderFunc) RecordCall(record CallRecord) { f(record) }

type usageRecorderFunc func(record UsageRecord)

func (f usageRecorderFunc) RecordUsage(record UsageRecord) { f(record) }
