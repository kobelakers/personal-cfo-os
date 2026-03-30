package memory

import (
	"context"
	"fmt"

	"github.com/kobelakers/personal-cfo-os/internal/skills"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

type SkillOutcomeMemory struct {
	WorkflowID            string                      `json:"workflow_id"`
	TaskID                string                      `json:"task_id"`
	TraceID               string                      `json:"trace_id,omitempty"`
	SkillFamily           string                      `json:"skill_family"`
	SkillVersion          string                      `json:"skill_version"`
	RecipeID              string                      `json:"recipe_id"`
	AnomalyCodes          []string                    `json:"anomaly_codes,omitempty"`
	InterventionIntensity skills.InterventionIntensity `json:"intervention_intensity,omitempty"`
	FinalRuntimeState     string                      `json:"final_runtime_state"`
	GovernanceOutcome     string                      `json:"governance_outcome,omitempty"`
	ApprovalOutcome       string                      `json:"approval_outcome,omitempty"`
	ArtifactRefs          []string                    `json:"artifact_refs,omitempty"`
	MemoryRefs            []string                    `json:"memory_refs,omitempty"`
}

type SkillSelectionMemoryContext struct {
	WorkflowID string `json:"workflow_id,omitempty"`
	TaskID     string `json:"task_id,omitempty"`
	TraceID    string `json:"trace_id,omitempty"`
	Consumer   string `json:"consumer,omitempty"`
}

func SkillSelectionMemoryRecords(records []MemoryRecord) []skills.ProceduralMemoryContextRecord {
	result := make([]skills.ProceduralMemoryContextRecord, 0, len(records))
	for _, item := range records {
		facts := make(map[string]string, len(item.Facts))
		for _, fact := range item.Facts {
			if fact.Key == "" {
				continue
			}
			facts[fact.Key] = fact.Value
		}
		result = append(result, skills.ProceduralMemoryContextRecord{
			ID:    item.ID,
			Kind:  string(item.Kind),
			Facts: facts,
		})
	}
	return result
}

func (s WorkflowMemoryService) WriteBehaviorSkillOutcome(
	ctx context.Context,
	spec taskspec.TaskSpec,
	outcome SkillOutcomeMemory,
) ([]string, error) {
	now := s.now()
	facts := []MemoryFact{
		{Key: "skill_family", Value: outcome.SkillFamily},
		{Key: "skill_version", Value: outcome.SkillVersion},
		{Key: "recipe_id", Value: outcome.RecipeID},
		{Key: "final_runtime_state", Value: string(outcome.FinalRuntimeState)},
	}
	facts = appendNonEmptyMemoryFact(facts, "anomaly_codes", joinFacts(outcome.AnomalyCodes))
	facts = appendNonEmptyMemoryFact(facts, "intervention_intensity", string(outcome.InterventionIntensity))
	facts = appendNonEmptyMemoryFact(facts, "governance_outcome", outcome.GovernanceOutcome)
	facts = appendNonEmptyMemoryFact(facts, "approval_outcome", outcome.ApprovalOutcome)
	facts = appendNonEmptyMemoryFact(facts, "artifact_refs", joinFacts(outcome.ArtifactRefs))
	record := MemoryRecord{
		ID:      outcome.WorkflowID + "-skill-outcome-" + outcome.RecipeID,
		Kind:    MemoryKindProcedural,
		Summary: fmt.Sprintf("Behavior intervention executed %s/%s and ended in runtime state %s.", outcome.SkillFamily, outcome.RecipeID, outcome.FinalRuntimeState),
		Facts:   facts,
		Source: MemorySource{
			TaskID:     spec.ID,
			WorkflowID: outcome.WorkflowID,
			TraceID:    outcome.TraceID,
			Actor:      "behavior_workflow",
		},
		Confidence: MemoryConfidence{Score: 0.94, Rationale: "durable skill outcome written by behavior workflow"},
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := s.writeRecord(ctx, record); err != nil {
		return nil, err
	}
	return []string{record.ID}, nil
}

func (s WorkflowMemoryService) writeRecord(ctx context.Context, record MemoryRecord) error {
	if s.Writer == nil {
		return nil
	}
	if s.Gate != nil {
		if err := s.Gate.AllowWrite(ctx, record); err != nil {
			return err
		}
	}
	return s.Writer.Write(ctx, record)
}

func joinFacts(items []string) string {
	if len(items) == 0 {
		return ""
	}
	result := items[0]
	for i := 1; i < len(items); i++ {
		result += "," + items[i]
	}
	return result
}

func appendNonEmptyMemoryFact(facts []MemoryFact, key string, value string) []MemoryFact {
	if value == "" {
		return facts
	}
	return append(facts, MemoryFact{Key: key, Value: value})
}
