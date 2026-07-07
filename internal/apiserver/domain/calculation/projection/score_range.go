package projection

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"

// ScoreRangeProjection 是no-op 投影 用于 因子_计分 models whose。
// interpretation 已经 embedded in scale 计分。
type ScoreRangeProjection struct{}

func (ScoreRangeProjection) Apply(result *calculation.Result) *calculation.Result {
	return result
}
