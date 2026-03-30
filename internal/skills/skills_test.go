package skills

import (
	"testing"

	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

func TestDefaultBehaviorSkillCatalogValidates(t *testing.T) {
	catalog, err := DefaultBehaviorSkillCatalog()
	if err != nil {
		t.Fatalf("default behavior skill catalog: %v", err)
	}
	if len(catalog.Families()) != 3 {
		t.Fatalf("expected 3 behavior skill families, got %d", len(catalog.Families()))
	}
}

func TestDeterministicBehaviorSkillSelectorEscalatesFromProceduralMemory(t *testing.T) {
	catalog, err := DefaultBehaviorSkillCatalog()
	if err != nil {
		t.Fatalf("default behavior skill catalog: %v", err)
	}
	selector := DeterministicBehaviorSkillSelector{Catalog: catalog}
	selection, err := selector.Select(SelectionInput{
		Task:            taskspec.TaskSpec{UserIntentType: taskspec.UserIntentBehaviorIntervention},
		AllowedFamilies: []string{string(SkillFamilyDiscretionaryGuardrail)},
		CurrentState: state.FinancialWorldState{
			CashflowState: state.CashflowState{MonthlyNetIncomeCents: -1000},
			BehaviorState: state.BehaviorState{LateNightSpendingFrequency: 0.5},
		},
		ProceduralMemories: []ProceduralMemoryContextRecord{{
			ID:   "proc-1",
			Kind: "procedural",
			Facts: map[string]string{
				"skill_family": string(SkillFamilyDiscretionaryGuardrail),
				"recipe_id":    "soft_nudge.v1",
			},
		}},
	})
	if err != nil {
		t.Fatalf("select behavior skill: %v", err)
	}
	if selection.RecipeID != "hard_cap.v1" {
		t.Fatalf("expected hard_cap.v1 after repeated anomaly + procedural memory, got %q", selection.RecipeID)
	}
	if len(selection.MemoryRefs) == 0 {
		t.Fatalf("expected selection to carry procedural memory refs")
	}
}
