package governance

import "github.com/kobelakers/personal-cfo-os/internal/analysis"

func DisclosureReady(recommendations []analysis.Recommendation) bool {
	for _, recommendation := range recommendations {
		if recommendationTypeSensitive(recommendation.Type) && len(recommendation.Caveats) == 0 {
			return false
		}
	}
	return true
}
