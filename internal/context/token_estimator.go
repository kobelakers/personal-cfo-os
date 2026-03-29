package context

type TokenEstimator interface {
	EstimateText(text string) int
}

type HeuristicTokenEstimator struct{}

func (HeuristicTokenEstimator) EstimateText(text string) int {
	if text == "" {
		return 0
	}
	runes := len([]rune(text))
	estimate := runes / 4
	if runes%4 != 0 {
		estimate++
	}
	if estimate < 1 {
		return 1
	}
	return estimate
}
