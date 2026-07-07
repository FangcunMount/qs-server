package factor

// ScoringParamsPayload is the shared JSON shape for strategy-specific scoring params.
type ScoringParamsPayload struct {
	CntOptionContents []string `json:"cnt_option_contents,omitempty"`
}

// DimensionRule is the shared draft/published payload dimension shape.
type DimensionRule struct {
	Code            string                `json:"code"`
	Title           string                `json:"title"`
	QuestionCodes   []string              `json:"question_codes"`
	ScoringStrategy string                `json:"scoring_strategy"`
	ScoringParams   *ScoringParamsPayload `json:"scoring_params,omitempty"`
	MaxScore        *float64              `json:"max_score,omitempty"`
	IsTotalScore    bool                  `json:"is_total_score,omitempty"`
	IsShow          bool                  `json:"is_show"`
	Role            string                `json:"role,omitempty"`
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
			IsTotalScore:    dimension.IsTotalScore,
			QuestionCodes:   append([]string(nil), dimension.QuestionCodes...),
			ScoringStrategy: dimension.ScoringStrategy,
			ScoringParams:   scoringParamsFromPayload(dimension.ScoringParams),
			MaxScore:        dimension.MaxScore,
			InterpretRules:  rulesByDimension[dimension.Code],
		})
	}
	return factors
}

func scoringParamsFromPayload(payload *ScoringParamsPayload) *ScoringParams {
	if payload == nil || len(payload.CntOptionContents) == 0 {
		return nil
	}
	return &ScoringParams{
		CntOptionContents: append([]string(nil), payload.CntOptionContents...),
	}
}
