package model

import (
	"context"
	"time"
)

type StaticResponder func(request ModelRequest) (ModelResponse, error)

// StaticChatModel is used for stable local demo/test evidence. It exercises the
// same chat/structured interface without binding 5B golden-path tests to a live
// provider.
type StaticChatModel struct {
	Provider  string
	Responder StaticResponder
}

func (m StaticChatModel) Generate(_ context.Context, request ModelRequest) (ModelResponse, error) {
	if m.Responder != nil {
		return m.Responder(request)
	}
	modelName := request.Model
	if modelName == "" {
		modelName = "static-mock-model"
	}
	return ModelResponse{
		Provider: m.Provider,
		Model:    modelName,
		Profile:  request.Profile,
		Content:  "{}",
		Usage: UsageStats{
			PromptTokens:     128,
			CompletionTokens: 96,
			TotalTokens:      224,
			EstimatedCostUSD: estimateCostUSD(modelName, request.Profile, 128, 96),
		},
		Latency: 15 * time.Millisecond,
	}, nil
}
