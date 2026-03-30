package behavior

import "fmt"

func (BehaviorValidator) Validate(output AnalysisOutput) error {
	if output.Summary == "" {
		return fmt.Errorf("behavior summary is required")
	}
	if output.SelectedSkill.Family == "" || output.SelectedSkill.RecipeID == "" {
		return fmt.Errorf("selected skill is required")
	}
	if len(output.Recommendations) == 0 {
		return fmt.Errorf("behavior recommendation is required")
	}
	for _, recommendation := range output.Recommendations {
		if len(recommendation.MetricRefs) == 0 || len(recommendation.EvidenceRefs) == 0 {
			return fmt.Errorf("behavior recommendation requires metric and evidence refs")
		}
	}
	return nil
}
