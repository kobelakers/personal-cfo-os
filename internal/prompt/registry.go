package prompt

import (
	"embed"
	"fmt"
	"path"

	"github.com/kobelakers/personal-cfo-os/internal/model"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

type PromptVersion string

type PromptTemplate struct {
	ID            string             `json:"id"`
	Version       PromptVersion      `json:"version"`
	ModelProfile  model.ModelProfile `json:"model_profile"`
	SystemPath    string             `json:"system_path"`
	UserPath      string             `json:"user_path"`
	RenderPolicy  PromptRenderPolicy `json:"render_policy"`
	systemContent string
	userContent   string
}

type PromptRegistry struct {
	templates map[string]PromptTemplate
}

func NewRegistry() (*PromptRegistry, error) {
	registry := &PromptRegistry{templates: make(map[string]PromptTemplate)}
	defaults := []PromptTemplate{
		{
			ID:           "planner.monthly_review.v1",
			Version:      "v1",
			ModelProfile: model.ModelProfilePlannerReasoning,
			SystemPath:   "templates/planner.monthly_review.v1.system.tmpl",
			UserPath:     "templates/planner.monthly_review.v1.user.tmpl",
			RenderPolicy: PromptRenderPolicy{ContextInjectionPolicy: "context_then_candidate_catalog"},
		},
		{
			ID:           "cashflow.monthly_review.v1",
			Version:      "v1",
			ModelProfile: model.ModelProfileCashflowFast,
			SystemPath:   "templates/cashflow.monthly_review.v1.system.tmpl",
			UserPath:     "templates/cashflow.monthly_review.v1.user.tmpl",
			RenderPolicy: PromptRenderPolicy{ContextInjectionPolicy: "context_then_grounded_metrics"},
		},
	}
	for _, item := range defaults {
		if err := registry.Register(item); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

func (r *PromptRegistry) Register(template PromptTemplate) error {
	if r == nil {
		return fmt.Errorf("prompt registry is nil")
	}
	if template.ID == "" {
		return fmt.Errorf("prompt template id is required")
	}
	systemContent, err := templateFS.ReadFile(path.Clean(template.SystemPath))
	if err != nil {
		return fmt.Errorf("read prompt system template %q: %w", template.SystemPath, err)
	}
	userContent, err := templateFS.ReadFile(path.Clean(template.UserPath))
	if err != nil {
		return fmt.Errorf("read prompt user template %q: %w", template.UserPath, err)
	}
	template.systemContent = string(systemContent)
	template.userContent = string(userContent)
	r.templates[template.ID] = template
	return nil
}

func (r *PromptRegistry) Lookup(id string) (PromptTemplate, error) {
	if r == nil {
		return PromptTemplate{}, fmt.Errorf("prompt registry is nil")
	}
	template, ok := r.templates[id]
	if !ok {
		return PromptTemplate{}, fmt.Errorf("unknown prompt template %q", id)
	}
	return template, nil
}
