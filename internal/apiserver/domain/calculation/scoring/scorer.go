package scoring

import "context"

// FactorScorer computes a single factor from question-level values.
type FactorScorer interface {
	ScoreFactor(ctx context.Context, factor Factor, values []float64) (float64, error)
}
