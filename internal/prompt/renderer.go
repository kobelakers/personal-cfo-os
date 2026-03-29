package prompt

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/model"
)

type PromptRenderer struct {
	Registry      *PromptRegistry
	TraceRecorder RenderTraceRecorder
}

type PromptRenderTrace struct {
	PromptID             string             `json:"prompt_id"`
	PromptVersion        PromptVersion      `json:"prompt_version"`
	ModelProfile         model.ModelProfile `json:"model_profile"`
	SelectedStateBlocks  []string           `json:"selected_state_blocks,omitempty"`
	SelectedMemoryIDs    []string           `json:"selected_memory_ids,omitempty"`
	SelectedEvidenceIDs  []string           `json:"selected_evidence_ids,omitempty"`
	SelectedSkillNames   []string           `json:"selected_skill_names,omitempty"`
	ExcludedBlockRefs    []string           `json:"excluded_block_refs,omitempty"`
	CompactionDecisions  []string           `json:"compaction_decisions,omitempty"`
	AppliedPolicy        string             `json:"applied_policy,omitempty"`
	RenderDecisions      []string           `json:"render_decisions,omitempty"`
	EstimatedInputTokens int                `json:"estimated_input_tokens"`
	RenderedAt           time.Time          `json:"rendered_at"`
}

type RenderedPrompt struct {
	ID           string             `json:"id"`
	Version      PromptVersion      `json:"version"`
	ModelProfile model.ModelProfile `json:"model_profile"`
	System       string             `json:"system"`
	User         string             `json:"user"`
	Trace        PromptRenderTrace  `json:"trace"`
}

func (p RenderedPrompt) Messages() []model.Message {
	return []model.Message{
		{Role: model.MessageRoleSystem, Content: p.System},
		{Role: model.MessageRoleUser, Content: p.User},
	}
}

func (r PromptRenderer) Render(id string, data any, traceInput PromptTraceInput) (RenderedPrompt, error) {
	if r.Registry == nil {
		return RenderedPrompt{}, fmt.Errorf("prompt renderer requires registry")
	}
	templateDef, err := r.Registry.Lookup(id)
	if err != nil {
		return RenderedPrompt{}, err
	}
	renderData, renderDecisions, err := applyRenderPolicy(templateDef, data)
	if err != nil {
		return RenderedPrompt{}, fmt.Errorf("apply render policy for %q: %w", id, err)
	}
	system, err := renderTemplate(templateDef.systemContent, renderData)
	if err != nil {
		return RenderedPrompt{}, fmt.Errorf("render prompt system template %q: %w", id, err)
	}
	user, err := renderTemplate(templateDef.userContent, renderData)
	if err != nil {
		return RenderedPrompt{}, fmt.Errorf("render prompt user template %q: %w", id, err)
	}
	trace := PromptRenderTrace{
		PromptID:             templateDef.ID,
		PromptVersion:        templateDef.Version,
		ModelProfile:         templateDef.ModelProfile,
		SelectedStateBlocks:  append([]string{}, traceInput.SelectedStateBlocks...),
		SelectedMemoryIDs:    append([]string{}, traceInput.SelectedMemoryIDs...),
		SelectedEvidenceIDs:  append([]string{}, traceInput.SelectedEvidenceIDs...),
		SelectedSkillNames:   append([]string{}, traceInput.SelectedSkillNames...),
		ExcludedBlockRefs:    append([]string{}, traceInput.ExcludedBlockRefs...),
		CompactionDecisions:  append([]string{}, traceInput.CompactionDecisions...),
		AppliedPolicy:        templateDef.RenderPolicy.ContextInjectionPolicy,
		RenderDecisions:      renderDecisions,
		EstimatedInputTokens: max(traceInput.EstimatedInputTokens, heuristicTokenEstimate(system)+heuristicTokenEstimate(user)),
		RenderedAt:           time.Now().UTC(),
	}
	if r.TraceRecorder != nil {
		r.TraceRecorder.RecordPromptRender(trace)
	}
	return RenderedPrompt{
		ID:           templateDef.ID,
		Version:      templateDef.Version,
		ModelProfile: templateDef.ModelProfile,
		System:       system,
		User:         user,
		Trace:        trace,
	}, nil
}

func applyRenderPolicy(templateDef PromptTemplate, data any) (map[string]any, []string, error) {
	renderData, err := normalizeTemplateData(data)
	if err != nil {
		return nil, nil, err
	}
	policy := templateDef.RenderPolicy.ContextInjectionPolicy
	decisions := []string{"applied_policy=" + policy}
	switch policy {
	case ContextInjectionPolicyContextThenCandidateCatalog:
		renderData["PolicyOrderedSections"] = joinPromptSections([]promptSection{
			{Heading: "上下文", Content: stringValue(renderData["ContextSummary"])},
			{Heading: "候选 blocks", Content: stringValue(renderData["CandidateBlocks"])},
			{Heading: "候选步骤", Content: stringValue(renderData["CandidateSteps"])},
		})
		decisions = append(decisions, "section_order=context->candidate_blocks->candidate_steps")
	case ContextInjectionPolicyContextThenGroundedMetrics:
		renderData["PolicyOrderedSections"] = joinPromptSections([]promptSection{
			{Heading: "上下文", Content: stringValue(renderData["ContextSummary"])},
			{Heading: "deterministic metrics", Content: stringValue(renderData["MetricsSummary"])},
			{Heading: "selected evidence", Content: stringValue(renderData["EvidenceSummary"])},
			{Heading: "selected memories", Content: stringValue(renderData["MemorySummary"])},
		})
		decisions = append(decisions, "section_order=context->grounded_metrics->evidence->memory")
	default:
		renderData["PolicyOrderedSections"] = joinPromptSections([]promptSection{
			{Heading: "上下文", Content: stringValue(renderData["ContextSummary"])},
		})
		decisions = append(decisions, "section_order=context")
	}
	return renderData, decisions, nil
}

type promptSection struct {
	Heading string
	Content string
}

func joinPromptSections(sections []promptSection) string {
	parts := make([]string, 0, len(sections))
	for _, section := range sections {
		content := strings.TrimSpace(section.Content)
		if section.Heading == "" || content == "" {
			continue
		}
		parts = append(parts, section.Heading+"：\n"+content)
	}
	return strings.Join(parts, "\n\n")
}

func normalizeTemplateData(data any) (map[string]any, error) {
	if existing, ok := data.(map[string]any); ok {
		cloned := make(map[string]any, len(existing))
		for key, value := range existing {
			cloned[key] = value
		}
		return cloned, nil
	}
	if data == nil {
		return map[string]any{}, nil
	}
	payload, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	result := make(map[string]any)
	if err := json.Unmarshal(payload, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

func renderTemplate(content string, data any) (string, error) {
	tpl, err := template.New("prompt").Parse(content)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func heuristicTokenEstimate(text string) int {
	if text == "" {
		return 0
	}
	runes := len([]rune(text))
	estimate := runes / 4
	if runes%4 != 0 {
		estimate++
	}
	return max(estimate, 1)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
