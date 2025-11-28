package interpretation

import (
	"fmt"
	"strconv"
)

// ==================== 阈值解读策略 ====================

// ThresholdStrategy 阈值解读策略
// 得分超过阈值则为指定风险等级
type ThresholdStrategy struct{}

// Interpret 执行阈值解读
func (s *ThresholdStrategy) Interpret(score float64, config *InterpretConfig) (*InterpretResult, error) {
	if config == nil || len(config.Rules) == 0 {
		return nil, ErrNoInterpretRules
	}

	// 从参数中获取阈值
	threshold := 0.0
	if thresholdStr, ok := config.Params["threshold"]; ok {
		if v, err := strconv.ParseFloat(thresholdStr, 64); err == nil {
			threshold = v
		}
	}

	// 默认使用第一条规则作为正常，第二条作为超阈值
	if len(config.Rules) < 2 {
		return nil, fmt.Errorf("threshold strategy requires at least 2 rules")
	}

	normalRule := config.Rules[0]
	highRiskRule := config.Rules[1]

	if score > threshold {
		return &InterpretResult{
			FactorCode:  config.FactorCode,
			Score:       score,
			RiskLevel:   highRiskRule.RiskLevel,
			Label:       highRiskRule.Label,
			Description: highRiskRule.Description,
			Suggestion:  highRiskRule.Suggestion,
		}, nil
	}

	return &InterpretResult{
		FactorCode:  config.FactorCode,
		Score:       score,
		RiskLevel:   normalRule.RiskLevel,
		Label:       normalRule.Label,
		Description: normalRule.Description,
		Suggestion:  normalRule.Suggestion,
	}, nil
}

// StrategyType 返回策略类型
func (s *ThresholdStrategy) StrategyType() StrategyType {
	return StrategyTypeThreshold
}

// ==================== 区间解读策略 ====================

// RangeStrategy 区间解读策略
// 根据得分所在区间确定风险等级
type RangeStrategy struct{}

// Interpret 执行区间解读
func (s *RangeStrategy) Interpret(score float64, config *InterpretConfig) (*InterpretResult, error) {
	if config == nil || len(config.Rules) == 0 {
		return nil, ErrNoInterpretRules
	}

	// 查找得分所在的区间
	for _, rule := range config.Rules {
		if rule.Contains(score) {
			return &InterpretResult{
				FactorCode:  config.FactorCode,
				Score:       score,
				RiskLevel:   rule.RiskLevel,
				Label:       rule.Label,
				Description: rule.Description,
				Suggestion:  rule.Suggestion,
			}, nil
		}
	}

	// 未匹配任何区间，使用默认（最后一条规则）
	lastRule := config.Rules[len(config.Rules)-1]
	return &InterpretResult{
		FactorCode:  config.FactorCode,
		Score:       score,
		RiskLevel:   lastRule.RiskLevel,
		Label:       lastRule.Label,
		Description: lastRule.Description,
		Suggestion:  lastRule.Suggestion,
	}, nil
}

// StrategyType 返回策略类型
func (s *RangeStrategy) StrategyType() StrategyType {
	return StrategyTypeRange
}
