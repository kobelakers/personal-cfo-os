package model

import "context"

type GeminiStubModel struct{}

func (GeminiStubModel) Generate(context.Context, ModelRequest) (ModelResponse, error) {
	return ModelResponse{}, &ProviderError{
		Category: ProviderErrorUnsupported,
		Message:  "gemini adapter is reserved for later phases and is not wired into the 5B golden path",
	}
}
