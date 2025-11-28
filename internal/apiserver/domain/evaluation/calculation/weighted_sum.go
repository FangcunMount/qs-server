package calculation

import (
	"encoding/json"
	"fmt"
)

// ==================== 加权求和策略 ====================

// WeightedSumStrategy 加权求和计分策略
// 按权重加权求和：sum(value_i * weight_i)
type WeightedSumStrategy struct{}

// Calculate 执行加权求和计分
func (s *WeightedSumStrategy) Calculate(values []float64, params map[string]string) (float64, error) {
	if len(values) == 0 {
		return 0, nil
	}

	// 获取权重配置
	weights, err := s.parseWeights(params, len(values))
	if err != nil {
		return 0, err
	}

	// 计算加权和
	var total float64
	for i, v := range values {
		total += v * weights[i]
	}
	return total, nil
}

// StrategyType 返回策略类型
func (s *WeightedSumStrategy) StrategyType() StrategyType {
	return StrategyTypeWeightedSum
}

// parseWeights 解析权重配置
func (s *WeightedSumStrategy) parseWeights(params map[string]string, count int) ([]float64, error) {
	weightsStr, ok := params[ParamKeyWeights]
	if !ok || weightsStr == "" {
		// 没有权重配置，使用默认权重 1.0
		weights := make([]float64, count)
		for i := range weights {
			weights[i] = 1.0
		}
		return weights, nil
	}

	var weights []float64
	if err := json.Unmarshal([]byte(weightsStr), &weights); err != nil {
		return nil, fmt.Errorf("invalid weights format: %w", err)
	}

	if len(weights) != count {
		return nil, fmt.Errorf("weights count (%d) does not match values count (%d)", len(weights), count)
	}

	return weights, nil
}
