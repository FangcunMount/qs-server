package calculation

import "math"

// ==================== 最大值策略 ====================

// MaxStrategy 最大值计分策略
// 取所有值的最大值
type MaxStrategy struct{}

// Calculate 执行最大值计分
func (s *MaxStrategy) Calculate(values []float64, params map[string]string) (float64, error) {
	if len(values) == 0 {
		return 0, nil
	}

	maxVal := math.Inf(-1)
	for _, v := range values {
		if v > maxVal {
			maxVal = v
		}
	}
	return maxVal, nil
}

// StrategyType 返回策略类型
func (s *MaxStrategy) StrategyType() StrategyType {
	return StrategyTypeMax
}

// ==================== 最小值策略 ====================

// MinStrategy 最小值计分策略
// 取所有值的最小值
type MinStrategy struct{}

// Calculate 执行最小值计分
func (s *MinStrategy) Calculate(values []float64, params map[string]string) (float64, error) {
	if len(values) == 0 {
		return 0, nil
	}

	minVal := math.Inf(1)
	for _, v := range values {
		if v < minVal {
			minVal = v
		}
	}
	return minVal, nil
}

// StrategyType 返回策略类型
func (s *MinStrategy) StrategyType() StrategyType {
	return StrategyTypeMin
}
