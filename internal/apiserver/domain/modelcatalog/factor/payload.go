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

// ParseFactorsFromDefinitionBody 从共享 payload parts 物化瘦领域 Factor。
func ParseFactorsFromDefinitionBody(dimensions []DimensionRule, interpretRules []InterpretRule) []Factor {
	return FactorsFromDefinitionDimensions(dimensions)
}

func childrenPolicyFromPayload(payload *ChildrenPolicyPayload) *ChildrenPolicy {
	if payload == nil {
		return nil
	}
	return &ChildrenPolicy{
		Strategy: ChildrenAggregationStrategy(payload.Strategy),
		Children: cloneStrings(payload.Children),
		Weights:  cloneWeights(payload.Weights),
	}
}

func cloneWeights(weights map[string]float64) map[string]float64 {
	if weights == nil {
		return nil
	}
	out := make(map[string]float64, len(weights))
	for key, value := range weights {
		out[key] = value
	}
	return out
}

func scoringParamsFromPayload(payload *ScoringParamsPayload) *ScoringParams {
	if payload == nil || len(payload.CntOptionContents) == 0 {
		return nil
	}
	return &ScoringParams{
		CntOptionContents: append([]string(nil), payload.CntOptionContents...),
	}
}
