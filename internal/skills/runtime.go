package skills

import "fmt"

type SkillRuntime interface {
	Resolve(selection SkillSelection) (SkillManifest, SkillRecipe, error)
}

type StaticSkillRuntime struct {
	Catalog SkillCatalog
}

func (r StaticSkillRuntime) Resolve(selection SkillSelection) (SkillManifest, SkillRecipe, error) {
	if r.Catalog == nil {
		return SkillManifest{}, SkillRecipe{}, fmt.Errorf("skill runtime requires catalog")
	}
	manifest, ok := r.Catalog.Manifest(selection.Family, selection.Version)
	if !ok {
		return SkillManifest{}, SkillRecipe{}, fmt.Errorf("unknown skill manifest %s:%s", selection.Family, selection.Version)
	}
	recipe, ok := recipeForID(manifest, selection.RecipeID)
	if !ok {
		return SkillManifest{}, SkillRecipe{}, fmt.Errorf("unknown recipe %q for %s:%s", selection.RecipeID, selection.Family, selection.Version)
	}
	return manifest, recipe, nil
}
