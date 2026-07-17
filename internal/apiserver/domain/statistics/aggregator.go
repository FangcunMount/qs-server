package statistics

// CompletionRate returns a percentage in the range implied by the inputs.
func CompletionRate(total, completed int64) float64 {
	if total == 0 {
		return 0
	}
	return float64(completed) / float64(total) * 100
}
