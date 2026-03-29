package prompt

import (
	"bytes"
	"fmt"
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
	system, err := renderTemplate(templateDef.systemContent, data)
	if err != nil {
		return RenderedPrompt{}, fmt.Errorf("render prompt system template %q: %w", id, err)
	}
	user, err := renderTemplate(templateDef.userContent, data)
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
