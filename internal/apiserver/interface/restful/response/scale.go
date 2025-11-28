package response

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/application/scale"
)

// ============= Scale Response =============

// ScaleResponse 量表响应
type ScaleResponse struct {
	Code                 string           `json:"code"`
	Title                string           `json:"title"`
	Description          string           `json:"description"`
	QuestionnaireCode    string           `json:"questionnaire_code"`
	QuestionnaireVersion string           `json:"questionnaire_version"`
	Status               string           `json:"status"`
	Factors              []FactorResponse `json:"factors,omitempty"`
}

// FactorResponse 因子响应
type FactorResponse struct {
	Code            string                  `json:"code"`
	Title           string                  `json:"title"`
	FactorType      string                  `json:"factor_type"`
	IsTotalScore    bool                    `json:"is_total_score"`
	QuestionCodes   []string                `json:"question_codes"`
	ScoringStrategy string                  `json:"scoring_strategy"`
	ScoringParams   map[string]string       `json:"scoring_params,omitempty"`
	InterpretRules  []InterpretRuleResponse `json:"interpret_rules,omitempty"`
}

// InterpretRuleResponse 解读规则响应
type InterpretRuleResponse struct {
	MinScore   float64 `json:"min_score"`
	MaxScore   float64 `json:"max_score"`
	RiskLevel  string  `json:"risk_level"`
	Conclusion string  `json:"conclusion"`
	Suggestion string  `json:"suggestion"`
}

// ScaleListResponse 量表列表响应
type ScaleListResponse struct {
	Scales     []ScaleResponse `json:"scales"`
	TotalCount int64           `json:"total_count"`
	Page       int             `json:"page"`
	PageSize   int             `json:"page_size"`
}

// ============= Converters =============

// NewScaleResponse 从 ScaleResult 创建 ScaleResponse
func NewScaleResponse(result *scale.ScaleResult) *ScaleResponse {
	if result == nil {
		return nil
	}

	factors := make([]FactorResponse, 0, len(result.Factors))
	for _, f := range result.Factors {
		factors = append(factors, newFactorResponse(f))
	}

	return &ScaleResponse{
		Code:                 result.Code,
		Title:                result.Title,
		Description:          result.Description,
		QuestionnaireCode:    result.QuestionnaireCode,
		QuestionnaireVersion: result.QuestionnaireVersion,
		Status:               result.Status,
		Factors:              factors,
	}
}

// newFactorResponse 从 FactorResult 创建 FactorResponse
func newFactorResponse(result scale.FactorResult) FactorResponse {
	rules := make([]InterpretRuleResponse, 0, len(result.InterpretRules))
	for _, r := range result.InterpretRules {
		rules = append(rules, InterpretRuleResponse{
			MinScore:   r.MinScore,
			MaxScore:   r.MaxScore,
			RiskLevel:  r.RiskLevel,
			Conclusion: r.Conclusion,
			Suggestion: r.Suggestion,
		})
	}

	return FactorResponse{
		Code:            result.Code,
		Title:           result.Title,
		FactorType:      result.FactorType,
		IsTotalScore:    result.IsTotalScore,
		QuestionCodes:   result.QuestionCodes,
		ScoringStrategy: result.ScoringStrategy,
		ScoringParams:   result.ScoringParams,
		InterpretRules:  rules,
	}
}

// NewScaleListResponse 从 ScaleListResult 创建 ScaleListResponse
func NewScaleListResponse(result *scale.ScaleListResult, page, pageSize int) *ScaleListResponse {
	if result == nil {
		return &ScaleListResponse{
			Scales:     []ScaleResponse{},
			TotalCount: 0,
			Page:       page,
			PageSize:   pageSize,
		}
	}

	scales := make([]ScaleResponse, 0, len(result.Items))
	for _, item := range result.Items {
		if resp := NewScaleResponse(item); resp != nil {
			scales = append(scales, *resp)
		}
	}

	return &ScaleListResponse{
		Scales:     scales,
		TotalCount: result.Total,
		Page:       page,
		PageSize:   pageSize,
	}
}
