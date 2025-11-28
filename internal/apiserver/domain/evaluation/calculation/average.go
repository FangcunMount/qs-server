package calculation

// ==================== 平均分策略 ====================

// AverageStrategy 平均分计分策略
// 计算所有值的平均值
type AverageStrategy struct{}

// Calculate 执行平均分计分
func (s *AverageStrategy) Calculate(values []float64, params map[string]string) (float64, error) {
	if len(values) == 0 {
		return 0, nil
	}

	var total float64
	for _, v := range values {
		total += v
	}
	return total / float64(len(values)), nil
}

// StrategyType 返回策略类型
func (s *AverageStrategy) StrategyType() StrategyType {
	return StrategyTypeAverage
}
