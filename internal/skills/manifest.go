package skills

import "fmt"

func (m SkillManifest) Validate() error {
	if m.Family == "" {
		return fmt.Errorf("skill family is required")
	}
	if m.Version == "" {
		return fmt.Errorf("skill version is required")
	}
	if m.Title == "" {
		return fmt.Errorf("skill title is required")
	}
	if m.Trigger.Intent == "" {
		return fmt.Errorf("skill trigger intent is required")
	}
	if m.ExpectedOutput.BlockKind == "" {
		return fmt.Errorf("skill expected output block kind is required")
	}
	if len(m.Recipes) == 0 {
		return fmt.Errorf("skill manifest requires at least one recipe")
	}
	seen := make(map[string]struct{}, len(m.Recipes))
	for _, recipe := range m.Recipes {
		if err := recipe.Validate(); err != nil {
			return fmt.Errorf("recipe %q invalid: %w", recipe.ID, err)
		}
		if _, ok := seen[recipe.ID]; ok {
			return fmt.Errorf("duplicate recipe id %q", recipe.ID)
		}
		seen[recipe.ID] = struct{}{}
	}
	return nil
}

func (r SkillRecipe) Validate() error {
	if r.ID == "" {
		return fmt.Errorf("recipe id is required")
	}
	if r.Title == "" {
		return fmt.Errorf("recipe title is required")
	}
	if r.InterventionIntensity == "" {
		return fmt.Errorf("recipe intervention intensity is required")
	}
	return nil
}
