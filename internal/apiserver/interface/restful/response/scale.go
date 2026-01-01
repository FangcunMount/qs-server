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
	Category             string           `json:"category,omitempty"`
	Stages               []string         `json:"stages,omitempty"`
	ApplicableAges       []string         `json:"applicable_ages,omitempty"`
	Reporters            []string         `json:"reporters,omitempty"`
	Tags                 []string         `json:"tags,omitempty"`
	QuestionnaireCode    string           `json:"questionnaire_code"`
	QuestionnaireVersion string           `json:"questionnaire_version"`
	Status               string           `json:"status"`
	Factors              []FactorResponse `json:"factors,omitempty"`
	CreatedBy            string           `json:"created_by"` // 创建人
	UpdatedBy            string           `json:"updated_by"` // 更新人
}

// FactorResponse 因子响应
type FactorResponse struct {
	Code            string                  `json:"code"`
	Title           string                  `json:"title"`
	FactorType      string                  `json:"factor_type"`
	IsTotalScore    bool                    `json:"is_total_score"`
	IsShow          bool                    `json:"is_show"` // 是否显示（用于报告中的维度展示）
	QuestionCodes   []string                `json:"question_codes"`
	ScoringStrategy string                  `json:"scoring_strategy"`
	ScoringParams   map[string]interface{}  `json:"scoring_params"`
	MaxScore        *float64                `json:"max_score,omitempty"`  // 最大分
	RiskLevel       string                  `json:"risk_level,omitempty"` // 因子级别的风险等级（从解读规则中提取）
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

// ScaleSummaryResponse 量表摘要响应（不包含因子详情）
type ScaleSummaryResponse struct {
	Code              string   `json:"code"`
	Title             string   `json:"title"`
	Description       string   `json:"description"`
	Category          string   `json:"category,omitempty"`
	Stages            []string `json:"stages,omitempty"`
	ApplicableAges    []string `json:"applicable_ages,omitempty"`
	Reporters         []string `json:"reporters,omitempty"`
	Tags              []string `json:"tags,omitempty"`
	QuestionnaireCode string   `json:"questionnaire_code"`
	Status            string   `json:"status"`
	CreatedBy         string   `json:"created_by"` // 创建人
	UpdatedBy         string   `json:"updated_by"` // 更新人
}

// ScaleSummaryListResponse 量表摘要列表响应
type ScaleSummaryListResponse struct {
	Scales     []ScaleSummaryResponse `json:"scales"`
	TotalCount int64                  `json:"total_count"`
	Page       int                    `json:"page"`
	PageSize   int                    `json:"page_size"`
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
		Category:             result.Category,
		Stages:               result.Stages,
		ApplicableAges:       result.ApplicableAges,
		Reporters:            result.Reporters,
		Tags:                 result.Tags,
		QuestionnaireCode:    result.QuestionnaireCode,
		QuestionnaireVersion: result.QuestionnaireVersion,
		Status:               result.Status,
		Factors:              factors,
		CreatedBy:            result.CreatedBy,
		UpdatedBy:            result.UpdatedBy,
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

	// 确保 scoring_params 不为 nil，至少返回空对象
	scoringParams := result.ScoringParams
	if scoringParams == nil {
		scoringParams = make(map[string]interface{})
	}

	return FactorResponse{
		Code:            result.Code,
		Title:           result.Title,
		FactorType:      result.FactorType,
		IsTotalScore:    result.IsTotalScore,
		IsShow:          result.IsShow,
		QuestionCodes:   result.QuestionCodes,
		ScoringStrategy: result.ScoringStrategy,
		ScoringParams:   scoringParams,
		MaxScore:        result.MaxScore,
		RiskLevel:       result.RiskLevel,
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

// NewScaleSummaryListResponse 从 ScaleSummaryListResult 创建摘要列表响应
func NewScaleSummaryListResponse(result *scale.ScaleSummaryListResult, page, pageSize int) *ScaleSummaryListResponse {
	if result == nil {
		return &ScaleSummaryListResponse{
			Scales:     []ScaleSummaryResponse{},
			TotalCount: 0,
			Page:       page,
			PageSize:   pageSize,
		}
	}

	scales := make([]ScaleSummaryResponse, 0, len(result.Items))
	for _, item := range result.Items {
		scales = append(scales, ScaleSummaryResponse{
			Code:              item.Code,
			Title:             item.Title,
			Description:       item.Description,
			Category:          item.Category,
			Stages:            item.Stages,
			ApplicableAges:    item.ApplicableAges,
			Reporters:         item.Reporters,
			Tags:              item.Tags,
			QuestionnaireCode: item.QuestionnaireCode,
			Status:            item.Status,
			CreatedBy:         item.CreatedBy,
			UpdatedBy:         item.UpdatedBy,
		})
	}

	return &ScaleSummaryListResponse{
		Scales:     scales,
		TotalCount: result.Total,
		Page:       page,
		PageSize:   pageSize,
	}
}

// FactorListResponse 因子列表响应
type FactorListResponse struct {
	Factors []FactorResponse `json:"factors"`
}

// NewFactorListResponse 从应用层 FactorResult 列表创建因子列表响应
func NewFactorListResponse(factors []scale.FactorResult) *FactorListResponse {
	factorResponses := make([]FactorResponse, 0, len(factors))
	for _, f := range factors {
		factorResponses = append(factorResponses, newFactorResponse(f))
	}

	return &FactorListResponse{
		Factors: factorResponses,
	}
}

// ScaleCategoriesResponse 量表分类响应
type ScaleCategoriesResponse struct {
	Categories     []CategoryResponse      `json:"categories"`
	Stages         []StageResponse         `json:"stages"`
	ApplicableAges []ApplicableAgeResponse `json:"applicable_ages"`
	Reporters      []ReporterResponse      `json:"reporters"`
	Tags           []TagResponse           `json:"tags"`
}

// CategoryResponse 类别响应
type CategoryResponse struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// StageResponse 阶段响应
type StageResponse struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// ApplicableAgeResponse 使用年龄响应
type ApplicableAgeResponse struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// ReporterResponse 填报人响应
type ReporterResponse struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// TagResponse 标签响应
type TagResponse struct {
	Value    string `json:"value"`
	Label    string `json:"label"`
	Category string `json:"category"`
}
