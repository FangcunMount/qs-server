package scoring

import "context"

// FactorScorer 计算单个 因子 从 题目-等级 values。
// Implementations typically delegate 到 ruleengine.Scale因子corer。
type FactorScorer interface {
	ScoreFactor(ctx context.Context, factorCode string, values []float64, strategy string, params map[string]string) (float64, error)
}
