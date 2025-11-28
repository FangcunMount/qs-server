package interpretation

// ==================== 组合解读策略 ====================

// CompositeStrategyImpl 组合解读策略实现
// 支持多因子组合解读
type CompositeStrategyImpl struct{}

// InterpretMultiple 执行多因子组合解读
func (s *CompositeStrategyImpl) InterpretMultiple(scores []FactorScore, config *CompositeConfig) (*CompositeResult, error) {
	if config == nil || len(config.Rules) == 0 {
		return nil, ErrNoInterpretRules
	}

	// 构建因子得分映射
	scoreMap := make(map[string]float64)
	for _, fs := range scores {
		scoreMap[fs.FactorCode] = fs.Score
	}

	// 遍历规则，找到第一个匹配的
	for _, rule := range config.Rules {
		if s.matchRule(rule, scoreMap) {
			return &CompositeResult{
				RiskLevel:   rule.RiskLevel,
				Label:       rule.Label,
				Description: rule.Description,
				Suggestion:  rule.Suggestion,
				Details:     nil, // 可选：添加各因子详细解读
			}, nil
		}
	}

	// 未匹配任何规则，返回默认（无风险）
	return &CompositeResult{
		RiskLevel:   RiskLevelNone,
		Label:       "正常",
		Description: "未匹配任何风险条件",
		Suggestion:  "",
	}, nil
}

// matchRule 检查规则是否匹配
func (s *CompositeStrategyImpl) matchRule(rule CompositeRule, scoreMap map[string]float64) bool {
	if len(rule.Conditions) == 0 {
		return false
	}

	switch rule.Operator {
	case "and":
		// 所有条件都必须满足
		for _, cond := range rule.Conditions {
			if !s.matchCondition(cond, scoreMap) {
				return false
			}
		}
		return true
	case "or":
		// 任一条件满足即可
		for _, cond := range rule.Conditions {
			if s.matchCondition(cond, scoreMap) {
				return true
			}
		}
		return false
	default:
		// 默认使用 and
		for _, cond := range rule.Conditions {
			if !s.matchCondition(cond, scoreMap) {
				return false
			}
		}
		return true
	}
}

// matchCondition 检查单个条件是否匹配
func (s *CompositeStrategyImpl) matchCondition(cond FactorCondition, scoreMap map[string]float64) bool {
	score, ok := scoreMap[cond.FactorCode]
	if !ok {
		return false // 因子不存在，条件不匹配
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

// StrategyType 返回策略类型
func (s *CompositeStrategyImpl) StrategyType() StrategyType {
	return StrategyTypeComposite
}
