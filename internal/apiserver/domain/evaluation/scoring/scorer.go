package scoring

import "context"

// FactorScorer scores a single factor from item-level values.
// Implementations typically delegate to ruleengine.ScaleFactorScorer.
type FactorScorer interface {
	ScoreFactor(ctx context.Context, factorCode string, values []float64, strategy string, params map[string]string) (float64, error)
}
