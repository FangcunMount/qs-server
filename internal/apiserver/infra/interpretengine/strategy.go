package interpretengine

import (
	"fmt"
	"strconv"

	"github.com/FangcunMount/qs-server/internal/apiserver/port/interpretengine"
)

type strategy interface {
	Interpret(score float64, config *interpretengine.Config) (*interpretengine.Result, error)
	StrategyType() interpretengine.StrategyType
}

type compositeStrategy interface {
	InterpretMultiple(scores []interpretengine.FactorScore, config *interpretengine.CompositeConfig) (*interpretengine.CompositeResult, error)
	StrategyType() interpretengine.StrategyType
}

type thresholdStrategy struct{}

func (s thresholdStrategy) Interpret(score float64, config *interpretengine.Config) (*interpretengine.Result, error) {
	if config == nil || len(config.Rules) == 0 {
		return nil, errNoInterpretRules
	}

	threshold := 0.0
	if thresholdStr, ok := config.Params["threshold"]; ok {
		if v, err := strconv.ParseFloat(thresholdStr, 64); err == nil {
			threshold = v
		}
	}

	if len(config.Rules) < 2 {
		return nil, fmt.Errorf("threshold strategy requires at least 2 rules")
	}

	normalRule := config.Rules[0]
	highRiskRule := config.Rules[1]
	if score > threshold {
		return resultFromRule(score, config.FactorCode, highRiskRule), nil
	}
	return resultFromRule(score, config.FactorCode, normalRule), nil
}

func (s thresholdStrategy) StrategyType() interpretengine.StrategyType {
	return interpretengine.StrategyTypeThreshold
}

type rangeStrategy struct{}

func (s rangeStrategy) Interpret(score float64, config *interpretengine.Config) (*interpretengine.Result, error) {
	if config == nil || len(config.Rules) == 0 {
		return nil, errNoInterpretRules
	}

	for _, rule := range config.Rules {
		if rule.Contains(score) {
			return resultFromRule(score, config.FactorCode, rule), nil
		}
	}

	lastRule := config.Rules[len(config.Rules)-1]
	return resultFromRule(score, config.FactorCode, lastRule), nil
}

func (s rangeStrategy) StrategyType() interpretengine.StrategyType {
	return interpretengine.StrategyTypeRange
}

type compositeStrategyImpl struct{}

func (s compositeStrategyImpl) InterpretMultiple(scores []interpretengine.FactorScore, config *interpretengine.CompositeConfig) (*interpretengine.CompositeResult, error) {
	if config == nil || len(config.Rules) == 0 {
		return nil, errNoInterpretRules
	}

	scoreMap := make(map[string]float64, len(scores))
	for _, fs := range scores {
		scoreMap[fs.FactorCode] = fs.Score
	}

	for _, rule := range config.Rules {
		if s.matchRule(rule, scoreMap) {
			return &interpretengine.CompositeResult{
				RiskLevel:   rule.RiskLevel,
				Label:       rule.Label,
				Description: rule.Description,
				Suggestion:  rule.Suggestion,
			}, nil
		}
	}

	return &interpretengine.CompositeResult{
		RiskLevel:   "none",
		Label:       "正常",
		Description: "未匹配任何风险条件",
		Suggestion:  "",
	}, nil
}

func (s compositeStrategyImpl) StrategyType() interpretengine.StrategyType {
	return interpretengine.StrategyTypeComposite
}

func (s compositeStrategyImpl) matchRule(rule interpretengine.CompositeRuleSpec, scoreMap map[string]float64) bool {
	if len(rule.Conditions) == 0 {
		return false
	}

	switch rule.Operator {
	case "and":
		for _, cond := range rule.Conditions {
			if !s.matchCondition(cond, scoreMap) {
				return false
			}
		}
		return true
	case "or":
		for _, cond := range rule.Conditions {
			if s.matchCondition(cond, scoreMap) {
				return true
			}
		}
		return false
	default:
		for _, cond := range rule.Conditions {
			if !s.matchCondition(cond, scoreMap) {
				return false
			}
		}
		return true
	}
}

func (s compositeStrategyImpl) matchCondition(cond interpretengine.FactorCondition, scoreMap map[string]float64) bool {
	score, ok := scoreMap[cond.FactorCode]
	if !ok {
		return false
	}

	switch cond.Operator {
	case ">":
		return score > cond.Value
	case ">=":
		return score >= cond.Value
	case "<":
		return score < cond.Value
	case "<=":
		return score <= cond.Value
	case "==":
		return score == cond.Value
	case "between":
		return score >= cond.Value && score <= cond.MaxValue
	default:
		return false
	}
}

func resultFromRule(score float64, factorCode string, rule interpretengine.RuleSpec) *interpretengine.Result {
	return &interpretengine.Result{
		FactorCode:  factorCode,
		Score:       score,
		RiskLevel:   rule.RiskLevel,
		Label:       rule.Label,
		Description: rule.Description,
		Suggestion:  rule.Suggestion,
	}
}
