package planning

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	contextview "github.com/kobelakers/personal-cfo-os/internal/context"
	"github.com/kobelakers/personal-cfo-os/internal/model"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/prompt"
	"github.com/kobelakers/personal-cfo-os/internal/structured"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

type CandidatePlanCatalog struct {
	Plan ExecutionPlan
}

type CandidatePlanCatalogBuilder struct {
	Planner *DeterministicPlanner
}

func (b CandidatePlanCatalogBuilder) Build(spec taskspec.TaskSpec, slice contextview.ContextSlice, workflowID string) CandidatePlanCatalog {
	planner := b.Planner
	if planner == nil {
		planner = &DeterministicPlanner{}
	}
	return CandidatePlanCatalog{Plan: planner.CreatePlan(spec, slice, workflowID)}
}

type DeterministicFallbackPlanner struct {
	Planner *DeterministicPlanner
}

func (p DeterministicFallbackPlanner) CreatePlan(spec taskspec.TaskSpec, slice contextview.ContextSlice, workflowID string) ExecutionPlan {
	planner := p.Planner
	if planner == nil {
		planner = &DeterministicPlanner{}
	}
	return planner.CreatePlan(spec, slice, workflowID)
}

type PlannerStructuredCandidate struct {
	PlanSummary            string                    `json:"plan_summary"`
	Rationale              string                    `json:"rationale"`
	VerificationFocusNotes []string                  `json:"verification_focus_notes,omitempty"`
	BlockOrder             []string                  `json:"block_order"`
	StepEmphasis           []PlannerStepEmphasisNote `json:"step_emphasis,omitempty"`
}

type PlannerStepEmphasisNote struct {
	StepID   string `json:"step_id"`
	Emphasis string `json:"emphasis"`
}

type PlannerStructuredSchema struct {
	AllowedBlockIDs []string
	AllowedStepIDs  []string
}

func (s PlannerStructuredSchema) Schema() structured.Schema[PlannerStructuredCandidate] {
	return structured.Schema[PlannerStructuredCandidate]{
		Name:   "PlannerPlanSchema",
		Parser: structured.JSONParser[PlannerStructuredCandidate]{},
		Validator: structured.ValidatorFunc[PlannerStructuredCandidate](func(value PlannerStructuredCandidate) []string {
			return s.Validate(value)
		}),
	}
}

func (s PlannerStructuredSchema) Validate(value PlannerStructuredCandidate) []string {
	diagnostics := make([]string, 0)
	if strings.TrimSpace(value.PlanSummary) == "" {
		diagnostics = append(diagnostics, "plan_summary is required")
	}
	if strings.TrimSpace(value.Rationale) == "" {
		diagnostics = append(diagnostics, "rationale is required")
	}
	if len(value.BlockOrder) != len(s.AllowedBlockIDs) {
		diagnostics = append(diagnostics, "block_order must include every allowed block exactly once")
	} else {
		remaining := make(map[string]int, len(s.AllowedBlockIDs))
		for _, id := range s.AllowedBlockIDs {
			remaining[id]++
		}
		for _, id := range value.BlockOrder {
			remaining[id]--
		}
		for id, count := range remaining {
			if count != 0 {
				diagnostics = append(diagnostics, fmt.Sprintf("block_order mismatch for %s", id))
			}
		}
	}
	if len(value.StepEmphasis) > 0 {
		allowed := make(map[string]struct{}, len(s.AllowedStepIDs))
		for _, id := range s.AllowedStepIDs {
			allowed[id] = struct{}{}
		}
		for _, item := range value.StepEmphasis {
			if _, ok := allowed[item.StepID]; !ok {
				diagnostics = append(diagnostics, fmt.Sprintf("unknown step_id %s", item.StepID))
			}
			if strings.TrimSpace(item.Emphasis) == "" {
				diagnostics = append(diagnostics, fmt.Sprintf("empty emphasis for step_id %s", item.StepID))
			}
		}
	}
	return diagnostics
}

type PlanCompiler struct{}

func (PlanCompiler) Compile(base ExecutionPlan, candidate PlannerStructuredCandidate, renderTrace prompt.PromptRenderTrace, generation model.StructuredGenerationResult, pipelineResult structured.Result[PlannerStructuredCandidate]) ExecutionPlan {
	blockIndex := make(map[string]ExecutionBlock, len(base.Blocks))
	for _, block := range base.Blocks {
		blockIndex[string(block.ID)] = block
	}
	compiledBlocks := make([]ExecutionBlock, 0, len(candidate.BlockOrder))
	for _, id := range candidate.BlockOrder {
		if block, ok := blockIndex[id]; ok {
			compiledBlocks = append(compiledBlocks, block)
		}
	}
	if len(compiledBlocks) == 0 {
		compiledBlocks = base.Blocks
	}
	emphasisByStep := make(map[string]string, len(candidate.StepEmphasis))
	for _, item := range candidate.StepEmphasis {
		emphasisByStep[item.StepID] = item.Emphasis
	}
	compiledSteps := make([]PlanStep, 0, len(base.Steps))
	for _, step := range base.Steps {
		cloned := step
		if emphasis, ok := emphasisByStep[step.ID]; ok {
			cloned.Emphasis = emphasis
		}
		compiledSteps = append(compiledSteps, cloned)
	}
	compiled := base
	compiled.Blocks = compiledBlocks
	compiled.Steps = compiledSteps
	compiled.Summary = candidate.PlanSummary
	compiled.Rationale = candidate.Rationale
	compiled.VerificationFocusNotes = append([]string{}, candidate.VerificationFocusNotes...)
	compiled.Trace = &PlanTraceMetadata{
		PromptID:             renderTrace.PromptID,
		PromptVersion:        string(renderTrace.PromptVersion),
		ModelProfile:         string(renderTrace.ModelProfile),
		EstimatedInputTokens: renderTrace.EstimatedInputTokens,
		PromptTokens:         generation.Response.Usage.PromptTokens,
		CompletionTokens:     generation.Response.Usage.CompletionTokens,
		TotalTokens:          generation.Response.Usage.TotalTokens,
		EstimatedCostUSD:     generation.Response.Usage.EstimatedCostUSD,
		FallbackUsed:         pipelineResult.FallbackUsed,
		FallbackReason:       pipelineResult.FallbackReason,
		StructuredFailure:    string(pipelineResult.FailureCategory),
		GeneratedAt:          time.Now().UTC(),
	}
	return compiled
}

type ProviderBackedPlanner struct {
	PromptRenderer prompt.PromptRenderer
	Generator      model.StructuredGenerator
	TraceRecorder  structured.TraceRecorder
	CatalogBuilder CandidatePlanCatalogBuilder
	Compiler       PlanCompiler
	Fallback       DeterministicFallbackPlanner
	Now            func() time.Time
}

func (p ProviderBackedPlanner) CreatePlan(spec taskspec.TaskSpec, slice contextview.ContextSlice, workflowID string) ExecutionPlan {
	return p.CreatePlanWithContext(context.Background(), spec, slice, workflowID)
}

func (p ProviderBackedPlanner) CreatePlanWithContext(ctx context.Context, spec taskspec.TaskSpec, slice contextview.ContextSlice, workflowID string) ExecutionPlan {
	catalog := p.CatalogBuilder.Build(spec, slice, workflowID)
	if spec.UserIntentType != taskspec.UserIntentMonthlyReview {
		return p.Fallback.CreatePlan(spec, slice, workflowID)
	}
	allowedBlockIDs := make([]string, 0, len(catalog.Plan.Blocks))
	for _, block := range catalog.Plan.Blocks {
		allowedBlockIDs = append(allowedBlockIDs, string(block.ID))
	}
	allowedStepIDs := make([]string, 0, len(catalog.Plan.Steps))
	for _, step := range catalog.Plan.Steps {
		allowedStepIDs = append(allowedStepIDs, step.ID)
	}
	rendered, err := p.PromptRenderer.Render("planner.monthly_review.v1", struct {
		Goal            string
		CandidateBlocks string
		CandidateSteps  string
		ContextSummary  string
	}{
		Goal:            spec.Goal,
		CandidateBlocks: candidateBlocksText(catalog.Plan.Blocks),
		CandidateSteps:  candidateStepsText(catalog.Plan.Steps),
		ContextSummary:  contextview.ContextSummary(slice),
	}, prompt.PromptTraceInput{
		SelectedStateBlocks:  selectedStateBlockNames(slice),
		SelectedMemoryIDs:    append([]string{}, slice.MemoryIDs...),
		SelectedEvidenceIDs:  evidenceIDsToStrings(slice.EvidenceIDs),
		SelectedSkillNames:   contextview.SkillNames(slice),
		ExcludedBlockRefs:    contextview.ExcludedBlockRefs(slice),
		CompactionDecisions:  contextview.CompactionNotes(slice),
		EstimatedInputTokens: slice.BudgetDecision.EstimatedInputTokens,
	})
	if err != nil {
		plan := p.Fallback.CreatePlan(spec, slice, workflowID)
		attachPlannerFallbackTrace(&plan, "prompt_render_failed", err.Error(), "planner.monthly_review.v1", string(model.ModelProfilePlannerReasoning))
		return plan
	}
	pipeline := structured.Pipeline[PlannerStructuredCandidate]{
		Schema:        PlannerStructuredSchema{AllowedBlockIDs: allowedBlockIDs, AllowedStepIDs: allowedStepIDs}.Schema(),
		Generator:     p.Generator,
		TraceRecorder: p.TraceRecorder,
		RepairPolicy:  structured.DefaultRepairPolicy(),
		FallbackPolicy: structured.FallbackPolicy[PlannerStructuredCandidate]{
			Name: "deterministic_catalog_order",
			Execute: func() (PlannerStructuredCandidate, error) {
				return PlannerStructuredCandidate{
					PlanSummary:            "deterministic fallback monthly review plan",
					Rationale:              "provider-backed planner output was unavailable or invalid, so deterministic monthly review ordering was used",
					VerificationFocusNotes: []string{"use deterministic block order and existing block-level verification hints"},
					BlockOrder:             allowedBlockIDs,
				}, nil
			},
		},
	}
	request := model.StructuredGenerationRequest{
		ModelRequest: model.ModelRequest{
			Profile:         model.ModelProfilePlannerReasoning,
			Messages:        rendered.Messages(),
			ResponseFormat:  model.ResponseFormat{Type: model.ResponseFormatJSONObject},
			MaxOutputTokens: 700,
			Temperature:     0.1,
			WorkflowID:      workflowID,
			TaskID:          spec.ID,
			TraceID:         workflowID,
			Agent:           "planner_agent",
			PromptID:        rendered.ID,
			PromptVersion:   string(rendered.Version),
		},
	}
	result, err := pipeline.Execute(ctx, request)
	if err != nil {
		plan := p.Fallback.CreatePlan(spec, slice, workflowID)
		attachPlannerFallbackTrace(&plan, string(structured.FailureCategoryFallbackFailed), err.Error(), rendered.ID, string(rendered.ModelProfile))
		return plan
	}
	return p.Compiler.Compile(catalog.Plan, result.Value, rendered.Trace, result.Generation, result)
}

func attachPlannerFallbackTrace(plan *ExecutionPlan, failureCategory string, reason string, promptID string, profile string) {
	if plan == nil {
		return
	}
	plan.Trace = &PlanTraceMetadata{
		PromptID:          promptID,
		ModelProfile:      profile,
		FallbackUsed:      true,
		FallbackReason:    reason,
		StructuredFailure: failureCategory,
		GeneratedAt:       time.Now().UTC(),
	}
}

func candidateBlocksText(blocks []ExecutionBlock) string {
	lines := make([]string, 0, len(blocks))
	for _, block := range blocks {
		payload, _ := json.Marshal(struct {
			ID                ExecutionBlockID   `json:"id"`
			Kind              ExecutionBlockKind `json:"kind"`
			AssignedRecipient string             `json:"assigned_recipient"`
			Goal              string             `json:"goal"`
		}{
			ID:                block.ID,
			Kind:              block.Kind,
			AssignedRecipient: block.AssignedRecipient,
			Goal:              block.Goal,
		})
		lines = append(lines, string(payload))
	}
	return strings.Join(lines, "\n")
}

func candidateStepsText(steps []PlanStep) string {
	lines := make([]string, 0, len(steps))
	for _, step := range steps {
		payload, _ := json.Marshal(step)
		lines = append(lines, string(payload))
	}
	return strings.Join(lines, "\n")
}

func evidenceIDsToStrings(ids []observation.EvidenceID) []string {
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		result = append(result, string(id))
	}
	return result
}

func selectedStateBlockNames(slice contextview.ContextSlice) []string {
	result := make([]string, 0, len(slice.StateBlocks))
	for _, block := range slice.StateBlocks {
		result = append(result, block.Name)
	}
	return result
}
