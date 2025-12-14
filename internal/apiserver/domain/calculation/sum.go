package calculation

// ==================== 求和策略 ====================

// SumStrategy 求和计分策略
// 将所有值相加
type SumStrategy struct{}

// Calculate 执行求和计分
func (s *SumStrategy) Calculate(values []float64, params map[string]string) (float64, error) {
	if len(values) == 0 {
		return 0, nil
	}

	var total float64
	for _, v := range values {
		total += v
	}
	return total, nil
}

// StrategyType 返回策略类型
func (s *SumStrategy) StrategyType() StrategyType {
	return StrategyTypeSum
}
