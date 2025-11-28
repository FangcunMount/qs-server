package calculation

// ==================== 计数策略 ====================

// CountStrategy 计数计分策略
// 返回值的数量
type CountStrategy struct{}

// Calculate 执行计数计分
func (s *CountStrategy) Calculate(values []float64, params map[string]string) (float64, error) {
	return float64(len(values)), nil
}

// StrategyType 返回策略类型
func (s *CountStrategy) StrategyType() StrategyType {
	return StrategyTypeCount
}

// ==================== 首值策略 ====================

// FirstStrategy 首值计分策略
// 返回第一个值
type FirstStrategy struct{}

// Calculate 执行首值计分
func (s *FirstStrategy) Calculate(values []float64, params map[string]string) (float64, error) {
	if len(values) == 0 {
		return 0, nil
	}
	return values[0], nil
}

// StrategyType 返回策略类型
func (s *FirstStrategy) StrategyType() StrategyType {
	return StrategyTypeFirst
}

// ==================== 末值策略 ====================

// LastStrategy 末值计分策略
// 返回最后一个值
type LastStrategy struct{}

// Calculate 执行末值计分
func (s *LastStrategy) Calculate(values []float64, params map[string]string) (float64, error) {
	if len(values) == 0 {
		return 0, nil
	}
	return values[len(values)-1], nil
}

// StrategyType 返回策略类型
func (s *LastStrategy) StrategyType() StrategyType {
	return StrategyTypeLast
}
