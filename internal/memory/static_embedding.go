package memory

import (
	"context"
	"time"
)

type StaticEmbeddingProvider struct {
	Dimensions    int
	CallRecorder  EmbeddingCallRecorder
	UsageRecorder EmbeddingUsageRecorder
}

func (p StaticEmbeddingProvider) Embed(ctx context.Context, text string) ([]float64, error) {
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

func (p StaticEmbeddingProvider) GenerateEmbedding(_ context.Context, request EmbeddingRequest) (EmbeddingResponse, error) {
	start := time.Now().UTC()
	provider := KeywordEmbeddingProvider{Dimensions: p.Dimensions}
	vector, err := provider.Embed(context.Background(), request.Input)
	if err != nil {
		return EmbeddingResponse{}, err
	}
	response := EmbeddingResponse{
		Provider:   "static-keyword",
		Model:      fallbackString(request.Model, "keyword-hash"),
		Vector:     vector,
		Dimensions: len(vector),
		Latency:    5 * time.Millisecond,
		Usage: EmbeddingUsageRecord{
			Provider:         "static-keyword",
			Model:            fallbackString(request.Model, "keyword-hash"),
			WorkflowID:       request.WorkflowID,
			TaskID:           request.TaskID,
			TraceID:          request.TraceID,
			Actor:            request.Actor,
			QueryID:          request.QueryID,
			InputTokens:      maxInt(len(tokenizeForIndexing(request.Input)), 1),
			EstimatedCostUSD: 0,
			RecordedAt:       time.Now().UTC(),
		},
	}
	if p.CallRecorder != nil {
		p.CallRecorder.RecordEmbeddingCall(EmbeddingCallRecord{
			Provider:    response.Provider,
			Model:       response.Model,
			WorkflowID:  request.WorkflowID,
			TaskID:      request.TaskID,
			TraceID:     request.TraceID,
			Actor:       request.Actor,
			QueryID:     request.QueryID,
			LatencyMS:   response.Latency.Milliseconds(),
			StartedAt:   start,
			CompletedAt: time.Now().UTC(),
		})
	}
	if p.UsageRecorder != nil {
		p.UsageRecorder.RecordEmbeddingUsage(response.Usage)
	}
	return response, nil
}
