package factor

// ScoringParamsPayload is the shared JSON shape for strategy-specific scoring params.
type ScoringParamsPayload struct {
	CntOptionContents []string `json:"cnt_option_contents,omitempty"`
}

// DimensionRule is the shared draft/published payload dimension shape.
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

// ChildrenPolicyPayload is the JSON shape for parent factor derivation rules.
type ChildrenPolicyPayload struct {
	Strategy string             `json:"strategy"`
	Children []string           `json:"children"`
	Weights  map[string]float64 `json:"weights,omitempty"`
}

// InterpretRule groups score ranges for one dimension code.
type InterpretRule struct {
	DimensionCode string           `json:"dimension_code"`
	Ranges        []ScoreRangeRule `json:"ranges"`
}

// ParseFactorsFromDefinitionBody materializes canonical factors from shared payload parts.
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
