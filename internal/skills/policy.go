package skills

func (m SkillManifest) RequiresApproval(recipeID string) bool {
	if recipeID == "" {
		return false
	}
	for _, item := range m.Policy.RequiresApprovalRecipes {
		if item == recipeID {
			return true
		}
	}
	recipe, ok := recipeForID(m, recipeID)
	return ok && recipe.ApprovalRequired
}
