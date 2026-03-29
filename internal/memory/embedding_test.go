package memory

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/model"
)

func TestOpenAIEmbeddingProviderRequiresModel(t *testing.T) {
	provider := NewOpenAIEmbeddingProvider(OpenAIEmbeddingConfig{
		APIKey:    "test-key",
		BaseURL:   "https://example.test/v1",
		Transport: staticTransport{},
	})
	_, err := provider.GenerateEmbedding(t.Context(), EmbeddingRequest{Input: "memory text"})
	if err == nil {
		t.Fatalf("expected config error when embedding model is missing")
	}
	providerErr, ok := err.(*EmbeddingProviderError)
	if !ok || providerErr.Category != EmbeddingErrorConfig {
		t.Fatalf("expected embedding config error, got %T %+v", err, err)
	}
}

func TestStaticEmbeddingProviderIsDeterministic(t *testing.T) {
	callLog := &EmbeddingCallLog{}
	usageLog := &EmbeddingUsageLog{}
	provider := StaticEmbeddingProvider{
		Dimensions:    12,
		CallRecorder:  callLog,
		UsageRecorder: usageLog,
	}
	first, err := provider.GenerateEmbedding(t.Context(), EmbeddingRequest{
		Model:      "static-embed-v1",
		Input:      "duplicate subscriptions memory",
		WorkflowID: "workflow-1",
		TaskID:     "task-1",
		TraceID:    "trace-1",
		Actor:      "memory_indexer",
		QueryID:    "query-1",
	})
	if err != nil {
		t.Fatalf("generate first static embedding: %v", err)
	}
	second, err := provider.GenerateEmbedding(t.Context(), EmbeddingRequest{
		Model:      "static-embed-v1",
		Input:      "duplicate subscriptions memory",
		WorkflowID: "workflow-1",
		TaskID:     "task-1",
		TraceID:    "trace-1",
		Actor:      "memory_indexer",
		QueryID:    "query-2",
	})
	if err != nil {
		t.Fatalf("generate second static embedding: %v", err)
	}
	if len(first.Vector) != len(second.Vector) {
		t.Fatalf("expected same vector dimensions, got %d vs %d", len(first.Vector), len(second.Vector))
	}
	for i := range first.Vector {
		if first.Vector[i] != second.Vector[i] {
			t.Fatalf("expected deterministic embedding output, got %v vs %v", first.Vector, second.Vector)
		}
	}
	if len(callLog.Records()) != 2 || len(usageLog.Records()) != 2 {
		t.Fatalf("expected call and usage trace for static provider, got calls=%d usage=%d", len(callLog.Records()), len(usageLog.Records()))
	}
}

func TestOpenAIEmbeddingProviderHTTPAndRetry(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			http.Error(w, "rate limited", http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"embedding":[1,0,0]}],"usage":{"prompt_tokens":6,"total_tokens":6}}`))
	}))
	defer server.Close()

	callLog := &EmbeddingCallLog{}
	usageLog := &EmbeddingUsageLog{}
	provider := NewOpenAIEmbeddingProvider(OpenAIEmbeddingConfig{
		APIKey:         "test-key",
		BaseURL:        server.URL,
		EndpointPath:   "",
		EmbeddingModel: "text-embedding-test",
		Transport:      model.HTTPTransport{Client: server.Client()},
		RetryPolicy: model.RetryPolicy{
			MaxAttempts:    2,
			InitialBackoff: time.Millisecond,
			MaxBackoff:     5 * time.Millisecond,
		},
		TimeoutPolicy: model.TimeoutPolicy{RequestTimeout: 2 * time.Second},
		CallRecorder:  callLog,
		UsageRecorder: usageLog,
	})

	response, err := provider.GenerateEmbedding(context.Background(), EmbeddingRequest{
		Input:      "subscription cleanup",
		WorkflowID: "workflow-1",
		TaskID:     "task-1",
		TraceID:    "trace-1",
		Actor:      "planner_agent",
		QueryID:    "query-1",
	})
	if err != nil {
		t.Fatalf("generate embedding through http: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("expected retry before success, got %d attempts", attempts)
	}
	if response.Provider == "" || response.Model != "text-embedding-test" || response.Dimensions != 3 {
		t.Fatalf("unexpected embedding response: %+v", response)
	}
	if len(callLog.Records()) != 2 {
		t.Fatalf("expected both failed and successful call records, got %+v", callLog.Records())
	}
	if callLog.Records()[0].ErrorCategory != EmbeddingErrorRateLimited {
		t.Fatalf("expected first attempt to be rate-limited, got %+v", callLog.Records()[0])
	}
	if len(usageLog.Records()) != 1 || usageLog.Records()[0].InputTokens != 6 {
		t.Fatalf("expected one embedding usage record, got %+v", usageLog.Records())
	}
}

type staticTransport struct{}

func (staticTransport) Do(_ context.Context, _ model.TransportRequest) (model.TransportResponse, error) {
	return model.TransportResponse{}, nil
}

type EmbeddingCallLog struct {
	records []EmbeddingCallRecord
}

func (l *EmbeddingCallLog) RecordEmbeddingCall(record EmbeddingCallRecord) {
	l.records = append(l.records, record)
}

func (l *EmbeddingCallLog) Records() []EmbeddingCallRecord {
	result := make([]EmbeddingCallRecord, len(l.records))
	copy(result, l.records)
	return result
}

type EmbeddingUsageLog struct {
	records []EmbeddingUsageRecord
}

func (l *EmbeddingUsageLog) RecordEmbeddingUsage(record EmbeddingUsageRecord) {
	l.records = append(l.records, record)
}

func (l *EmbeddingUsageLog) Records() []EmbeddingUsageRecord {
	result := make([]EmbeddingUsageRecord, len(l.records))
	copy(result, l.records)
	return result
}
