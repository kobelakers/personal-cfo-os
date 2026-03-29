package model

import "context"

type DefaultStructuredGenerator struct {
	Model ChatModel
}

func (g DefaultStructuredGenerator) Generate(ctx context.Context, request StructuredGenerationRequest) (StructuredGenerationResult, error) {
	if g.Model == nil {
		return StructuredGenerationResult{}, &ProviderError{
			Category: ProviderErrorConfig,
			Message:  "structured generator requires a chat model",
		}
	}
	response, err := g.Model.Generate(ctx, request.ModelRequest)
	if err != nil {
		return StructuredGenerationResult{}, err
	}
	return StructuredGenerationResult{
		Response: response,
		Content:  response.Content,
	}, nil
}
