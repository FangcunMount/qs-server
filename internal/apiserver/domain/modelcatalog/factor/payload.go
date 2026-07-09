package factor

// ScoringParamsPayload 是共享 JSON 结构 用于 strategy-特定 计分 params。
type ScoringParamsPayload struct {
	CntOptionContents []string `json:"cnt_option_contents,omitempty"`
}

// DimensionRule 是共享 draft/published 载荷 维度 结构。
type DimensionRule struct {
	Code            string                 `json:"code"`
	Title           string                 `json:"title"`
	ParentCode      string                 `json:"parent_code,omitempty"`
	SortOrder       int                    `json:"sort_order,omitempty"`
	Level           int                    `json:"level,omitempty"`
	QuestionCodes   []string               `json:"question_codes"`
	ScoringStrategy string                 `json:"scoring_strategy"`
	ScoringParams   *ScoringParamsPayload  `json:"scoring_params,omitempty"`
	MaxScore        *float64               `json:"max_score,omitempty"`
	IsTotalScore    bool                   `json:"is_total_score,omitempty"`
	IsShow          bool                   `json:"is_show"`
	Role            string                 `json:"role,omitempty"`
	ChildrenPolicy  *ChildrenPolicyPayload `json:"children_policy,omitempty"`
}

// ChildrenPolicyPayload 是JSON 结构 用于 父节点 因子 derivation rules。
type ChildrenPolicyPayload struct {
	Strategy string             `json:"strategy"`
	Children []string           `json:"children"`
	Weights  map[string]float64 `json:"weights,omitempty"`
}

// InterpretRule 分组score ranges 用于 一个维度 编码。
type InterpretRule struct {
	DimensionCode string           `json:"dimension_code"`
	Ranges        []ScoreRangeRule `json:"ranges"`
}

// ParseFactorsFromDefinitionBody 从共享 payload parts 物化兼容 FactorSnapshot DTO。
// 新领域逻辑优先使用 ParseFactorsFromDefinitionBodyAsFactors。
func ParseFactorsFromDefinitionBody(dimensions []DimensionRule, interpretRules []InterpretRule) []FactorSnapshot {
	rulesByDimension := make(map[string][]ScoreRangeRule, len(interpretRules))
	for _, rule := range interpretRules {
		rulesByDimension[rule.DimensionCode] = append([]ScoreRangeRule(nil), rule.Ranges...)
	}
	factors := make([]FactorSnapshot, 0, len(dimensions))
	for _, dimension := range dimensions {
		role := FactorRole(dimension.Role)
		if role != "" && !role.IsValid() {
			role = ""
		}
		factors = append(factors, FactorSnapshot{
			Code:            dimension.Code,
			Title:           dimension.Title,
			Role:            role,
			ParentCode:      dimension.ParentCode,
			SortOrder:       dimension.SortOrder,
			Level:           dimension.Level,
			IsTotalScore:    dimension.IsTotalScore,
			QuestionCodes:   append([]string(nil), dimension.QuestionCodes...),
			ScoringStrategy: dimension.ScoringStrategy,
			ScoringParams:   scoringParamsFromPayload(dimension.ScoringParams),
			MaxScore:        dimension.MaxScore,
			InterpretRules:  rulesByDimension[dimension.Code],
			ChildrenPolicy:  childrenPolicyFromPayload(dimension.ChildrenPolicy),
		})
	}
	return factors
}

// ParseFactorsFromDefinitionBodyAsFactors 从共享 payload parts 物化领域 Factor。
func ParseFactorsFromDefinitionBodyAsFactors(dimensions []DimensionRule, interpretRules []InterpretRule) []Factor {
	return FactorsFromSnapshots(ParseFactorsFromDefinitionBody(dimensions, interpretRules))
}

func childrenPolicyFromPayload(payload *ChildrenPolicyPayload) *ChildrenPolicy {
	if payload == nil {
		return nil
	}
	return &ChildrenPolicy{
		Strategy: ChildrenAggregationStrategy(payload.Strategy),
		Children: append([]string(nil), payload.Children...),
		Weights:  payload.Weights,
	}
}

func scoringParamsFromPayload(payload *ScoringParamsPayload) *ScoringParams {
	if payload == nil || len(payload.CntOptionContents) == 0 {
		return nil
	}
	return &ScoringParams{
		CntOptionContents: append([]string(nil), payload.CntOptionContents...),
	}
}
