package model

import "strings"

func estimateCostUSD(modelName string, profile ModelProfile, promptTokens int, completionTokens int) float64 {
	inputRate, outputRate := pricingForModel(modelName, profile)
	return (float64(promptTokens)/1_000_000.0)*inputRate + (float64(completionTokens)/1_000_000.0)*outputRate
}

func pricingForModel(modelName string, profile ModelProfile) (float64, float64) {
	name := strings.ToLower(modelName)
	switch {
	case strings.Contains(name, "gpt-5"):
		return 1.25, 10.00
	case strings.Contains(name, "gpt-4.1-mini"), strings.Contains(name, "mini"):
		return 0.40, 1.60
	case profile == ModelProfilePlannerReasoning:
		return 1.25, 10.00
	default:
		return 0.40, 1.60
	}
}
