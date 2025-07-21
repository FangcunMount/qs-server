package strategies

import (
	"context"
	"fmt"
	"math"

	"github.com/yshujie/questionnaire-scale/internal/evaluation-server/domain/calculation/rules"
)

// SumStrategy 求和计算策略
type SumStrategy struct {
	BaseStrategy
}

// NewSumStrategy 创建求和策略
func NewSumStrategy() *SumStrategy {
	return &SumStrategy{
		BaseStrategy: BaseStrategy{
			Name:        "sum",
			Description: "计算所有操作数的总和",
		},
	}
}

// Calculate 执行求和计算
func (s *SumStrategy) Calculate(ctx context.Context, operands []float64, rule *rules.CalculationRule) (*CalculationResult, error) {
	if err := s.Validate(operands, rule); err != nil {
		return nil, err
	}

	sum := 0.0
	for _, operand := range operands {
		sum += operand
	}

	result := NewCalculationResult(s.applyRounding(sum, rule), s.Name)
	result.SetMetadata("raw_sum", sum)
	result.SetMetadata("operand_count", len(operands))

	// 记录操作数信息
	for i, operand := range operands {
		result.AddOperandInfo(operand, 1.0, "", i)
	}

	return result, nil
}

// AverageStrategy 平均值计算策略
type AverageStrategy struct {
	BaseStrategy
}

// NewAverageStrategy 创建平均值策略
func NewAverageStrategy() *AverageStrategy {
	return &AverageStrategy{
		BaseStrategy: BaseStrategy{
			Name:        "average",
			Description: "计算所有操作数的平均值",
		},
	}
}

// Calculate 执行平均值计算
func (s *AverageStrategy) Calculate(ctx context.Context, operands []float64, rule *rules.CalculationRule) (*CalculationResult, error) {
	if err := s.Validate(operands, rule); err != nil {
		return nil, err
	}

	sum := 0.0
	for _, operand := range operands {
		sum += operand
	}

	average := sum / float64(len(operands))
	result := NewCalculationResult(s.applyRounding(average, rule), s.Name)
	result.SetMetadata("sum", sum)
	result.SetMetadata("raw_average", average)
	result.SetMetadata("operand_count", len(operands))

	// 记录操作数信息
	for i, operand := range operands {
		result.AddOperandInfo(operand, 1.0, "", i)
	}

	return result, nil
}

// MaxStrategy 最大值计算策略
type MaxStrategy struct {
	BaseStrategy
}

// NewMaxStrategy 创建最大值策略
func NewMaxStrategy() *MaxStrategy {
	return &MaxStrategy{
		BaseStrategy: BaseStrategy{
			Name:        "max",
			Description: "找出所有操作数中的最大值",
		},
	}
}

// Calculate 执行最大值计算
func (s *MaxStrategy) Calculate(ctx context.Context, operands []float64, rule *rules.CalculationRule) (*CalculationResult, error) {
	if err := s.Validate(operands, rule); err != nil {
		return nil, err
	}

	max := operands[0]
	maxIndex := 0

	for i, operand := range operands[1:] {
		if operand > max {
			max = operand
			maxIndex = i + 1
		}
	}

	result := NewCalculationResult(s.applyRounding(max, rule), s.Name)
	result.SetMetadata("max_index", maxIndex)
	result.SetMetadata("operand_count", len(operands))

	// 记录操作数信息
	for i, operand := range operands {
		result.AddOperandInfo(operand, 1.0, "", i)
	}

	return result, nil
}

// MinStrategy 最小值计算策略
type MinStrategy struct {
	BaseStrategy
}

// NewMinStrategy 创建最小值策略
func NewMinStrategy() *MinStrategy {
	return &MinStrategy{
		BaseStrategy: BaseStrategy{
			Name:        "min",
			Description: "找出所有操作数中的最小值",
		},
	}
}

// Calculate 执行最小值计算
func (s *MinStrategy) Calculate(ctx context.Context, operands []float64, rule *rules.CalculationRule) (*CalculationResult, error) {
	if err := s.Validate(operands, rule); err != nil {
		return nil, err
	}

	min := operands[0]
	minIndex := 0

	for i, operand := range operands[1:] {
		if operand < min {
			min = operand
			minIndex = i + 1
		}
	}

	result := NewCalculationResult(s.applyRounding(min, rule), s.Name)
	result.SetMetadata("min_index", minIndex)
	result.SetMetadata("operand_count", len(operands))

	// 记录操作数信息
	for i, operand := range operands {
		result.AddOperandInfo(operand, 1.0, "", i)
	}

	return result, nil
}

// OptionStrategy 选项计算策略
type OptionStrategy struct {
	BaseStrategy
}

// NewOptionStrategy 创建选项策略
func NewOptionStrategy() *OptionStrategy {
	return &OptionStrategy{
		BaseStrategy: BaseStrategy{
			Name:        "option",
			Description: "返回单个选项值，通常用于单选题",
		},
	}
}

// Validate 验证选项操作数
func (s *OptionStrategy) Validate(operands []float64, rule *rules.CalculationRule) error {
	if err := s.BaseStrategy.Validate(operands, rule); err != nil {
		return err
	}

	maxOperands := 1
	if rule.Config.MaxOperands > 0 {
		maxOperands = rule.Config.MaxOperands
	}

	if len(operands) > maxOperands {
		return NewCalculationError("",
			fmt.Sprintf("操作数数量 %d 超过最大允许数量 %d", len(operands), maxOperands),
			operands, s.Name)
	}

	return nil
}

// Calculate 执行选项计算
func (s *OptionStrategy) Calculate(ctx context.Context, operands []float64, rule *rules.CalculationRule) (*CalculationResult, error) {
	if err := s.Validate(operands, rule); err != nil {
		return nil, err
	}

	value := operands[0]
	result := NewCalculationResult(s.applyRounding(value, rule), s.Name)
	result.SetMetadata("operand_count", len(operands))

	// 记录操作数信息
	for i, operand := range operands {
		result.AddOperandInfo(operand, 1.0, "", i)
	}

	return result, nil
}

// WeightedStrategy 加权计算策略
type WeightedStrategy struct {
	BaseStrategy
}

// NewWeightedStrategy 创建加权策略
func NewWeightedStrategy() *WeightedStrategy {
	return &WeightedStrategy{
		BaseStrategy: BaseStrategy{
			Name:        "weighted",
			Description: "执行加权计算，支持加权平均和加权求和",
		},
	}
}

// Validate 验证加权操作数
func (s *WeightedStrategy) Validate(operands []float64, rule *rules.CalculationRule) error {
	if err := s.BaseStrategy.Validate(operands, rule); err != nil {
		return err
	}

	// 验证权重配置
	weights := rule.Config.Weights
	if len(weights) > 0 && len(weights) != len(operands) {
		return NewCalculationError("",
			fmt.Sprintf("权重数量 %d 与操作数数量 %d 不匹配", len(weights), len(operands)),
			operands, s.Name)
	}

	// 验证权重是否为正数
	for i, weight := range weights {
		if weight <= 0 {
			return NewCalculationError("",
				fmt.Sprintf("权重[%d] %f 必须为正数", i, weight),
				operands, s.Name)
		}
	}

	return nil
}

// Calculate 执行加权计算
func (s *WeightedStrategy) Calculate(ctx context.Context, operands []float64, rule *rules.CalculationRule) (*CalculationResult, error) {
	if err := s.Validate(operands, rule); err != nil {
		return nil, err
	}

	// 获取权重，如果没有配置则使用等权重
	weights := rule.Config.Weights
	if len(weights) == 0 {
		weights = make([]float64, len(operands))
		for i := range weights {
			weights[i] = 1.0
		}
	}

	weightedSum := 0.0
	totalWeight := 0.0

	for i, operand := range operands {
		weightedSum += operand * weights[i]
		totalWeight += weights[i]
	}

	// 默认计算加权平均值，可通过参数指定计算类型
	var finalValue float64
	calcType := "weighted_average"
	if rule.Params["calculation_type"] != nil {
		if ct, ok := rule.Params["calculation_type"].(string); ok {
			calcType = ct
		}
	}

	switch calcType {
	case "weighted_sum":
		finalValue = weightedSum
	case "weighted_average":
		fallthrough
	default:
		finalValue = weightedSum / totalWeight
	}

	result := NewCalculationResult(s.applyRounding(finalValue, rule), s.Name)
	result.SetMetadata("calculation_type", calcType)
	result.SetMetadata("weighted_sum", weightedSum)
	result.SetMetadata("total_weight", totalWeight)
	result.SetMetadata("weights", weights)
	result.SetMetadata("operand_count", len(operands))

	// 记录操作数信息
	for i, operand := range operands {
		result.AddOperandInfo(operand, weights[i], "", i)
	}

	return result, nil
}

// applyRounding 应用舍入规则（所有策略的公共方法）
func (s *BaseStrategy) applyRounding(value float64, rule *rules.CalculationRule) float64 {
	precision := rule.Config.Precision
	mode := rule.Config.RoundingMode

	multiplier := math.Pow(10, float64(precision))

	switch mode {
	case "ceil":
		return math.Ceil(value*multiplier) / multiplier
	case "floor":
		return math.Floor(value*multiplier) / multiplier
	case "round":
		fallthrough
	default:
		return math.Round(value*multiplier) / multiplier
	}
}
