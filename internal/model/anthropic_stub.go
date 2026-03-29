package model

import "context"

type AnthropicStubModel struct{}

func (AnthropicStubModel) Generate(context.Context, ModelRequest) (ModelResponse, error) {
	return ModelResponse{}, &ProviderError{
		Category: ProviderErrorUnsupported,
		Message:  "anthropic adapter is reserved for later phases and is not wired into the 5B golden path",
	}
}
