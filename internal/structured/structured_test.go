package structured

import (
	"context"
	"errors"
	"testing"

	"github.com/kobelakers/personal-cfo-os/internal/model"
)

func TestPipelineFallsBackAfterMalformedOutput(t *testing.T) {
	pipeline := Pipeline[struct {
		Value string `json:"value"`
	}]{
		Schema: Schema[struct {
			Value string `json:"value"`
		}]{
			Name: "test_schema",
			Parser: JSONParser[struct {
				Value string `json:"value"`
			}]{},
			Validator: ValidatorFunc[struct {
				Value string `json:"value"`
			}](func(v struct {
				Value string `json:"value"`
			}) []string { return nil }),
		},
		Generator: model.DefaultStructuredGenerator{
			Model: model.StaticChatModel{
				Responder: func(request model.ModelRequest) (model.ModelResponse, error) {
					return model.ModelResponse{Content: "{not-json"}, nil
				},
			},
		},
		FallbackPolicy: FallbackPolicy[struct {
			Value string `json:"value"`
		}]{
			Name: "fallback",
			Execute: func() (struct {
				Value string `json:"value"`
			}, error) {
				return struct {
					Value string `json:"value"`
				}{Value: "fallback"}, nil
			},
		},
	}
	result, err := pipeline.Execute(context.Background(), model.StructuredGenerationRequest{
		ModelRequest: model.ModelRequest{PromptID: "test"},
	})
	if err != nil {
		t.Fatalf("pipeline execute: %v", err)
	}
	if !result.FallbackUsed || result.Value.Value != "fallback" {
		t.Fatalf("expected fallback result, got %+v", result)
	}
}

func TestPipelineReturnsErrorWhenFallbackFails(t *testing.T) {
	pipeline := Pipeline[struct {
		Value string `json:"value"`
	}]{
		Schema: Schema[struct {
			Value string `json:"value"`
		}]{
			Name: "test_schema",
			Parser: JSONParser[struct {
				Value string `json:"value"`
			}]{},
			Validator: ValidatorFunc[struct {
				Value string `json:"value"`
			}](func(v struct {
				Value string `json:"value"`
			}) []string { return nil }),
		},
		Generator: model.DefaultStructuredGenerator{
			Model: model.StaticChatModel{
				Responder: func(request model.ModelRequest) (model.ModelResponse, error) {
					return model.ModelResponse{}, errors.New("transport failed")
				},
			},
		},
		FallbackPolicy: FallbackPolicy[struct {
			Value string `json:"value"`
		}]{
			Name: "fallback",
			Execute: func() (struct {
				Value string `json:"value"`
			}, error) {
				return struct {
					Value string `json:"value"`
				}{}, errors.New("fallback failed")
			},
		},
	}
	if _, err := pipeline.Execute(context.Background(), model.StructuredGenerationRequest{
		ModelRequest: model.ModelRequest{PromptID: "test"},
	}); err == nil {
		t.Fatalf("expected pipeline failure when fallback also fails")
	}
}
