package projection

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"

// ScoreRangeProjection is a no-op projection for factor_scoring models whose
// interpretation is already embedded in scale scoring.
type ScoreRangeProjection struct{}

func (ScoreRangeProjection) Apply(result *calculation.Result) *calculation.Result {
	return result
}
