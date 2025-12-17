package scale

import (
	"encoding/json"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
)

// ============= Result 定义 =============
// Results 用于应用服务层的输出结果

// ScaleResult 量表结果
type ScaleResult struct {
	Code                 string         // 量表编码
	Title                string         // 量表标题
	Description          string         // 量表描述
	QuestionnaireCode    string         // 关联的问卷编码
	QuestionnaireVersion string         // 关联的问卷版本
	Status               string         // 状态
	Factors              []FactorResult // 因子列表
}

// FactorResult 因子结果
type FactorResult struct {
	Code            string                 // 因子编码
	Title           string                 // 因子标题
	FactorType      string                 // 因子类型
	IsTotalScore    bool                   // 是否为总分因子
	QuestionCodes   []string               // 关联的题目编码列表
	ScoringStrategy string                 // 计分策略
	ScoringParams   map[string]interface{} // 计分参数
	InterpretRules  []InterpretRuleResult  // 解读规则列表
}

// InterpretRuleResult 解读规则结果
type InterpretRuleResult struct {
	MinScore   float64 // 最小分数（含）
	MaxScore   float64 // 最大分数（不含）
	RiskLevel  string  // 风险等级
	Conclusion string  // 结论文案
	Suggestion string  // 建议文案
}

// ScaleListResult 量表列表结果
type ScaleListResult struct {
	Items []*ScaleResult // 量表列表
	Total int64          // 总数
}

// ScaleSummaryResult 量表摘要结果（不包含因子列表，用于列表展示）
type ScaleSummaryResult struct {
	Code              string // 量表编码
	Title             string // 量表标题
	Description       string // 量表描述
	QuestionnaireCode string // 关联的问卷编码
	Status            string // 状态
}

// ScaleSummaryListResult 量表摘要列表结果
type ScaleSummaryListResult struct {
	Items []*ScaleSummaryResult // 量表摘要列表
	Total int64                 // 总数
}

// ============= Converter 转换器 =============

// toScaleResult 将领域模型转换为结果对象
func toScaleResult(m *scale.MedicalScale) *ScaleResult {
	if m == nil {
		return nil
	}

	result := &ScaleResult{
		Code:                 m.GetCode().String(),
		Title:                m.GetTitle(),
		Description:          m.GetDescription(),
		QuestionnaireCode:    m.GetQuestionnaireCode().String(),
		QuestionnaireVersion: m.GetQuestionnaireVersion(),
		Status:               m.GetStatus().String(),
		Factors:              make([]FactorResult, 0),
	}

	// 转换因子列表
	for _, factor := range m.GetFactors() {
		result.Factors = append(result.Factors, toFactorResult(factor))
	}

	return result
}

// toFactorResult 将因子领域模型转换为结果对象
func toFactorResult(f *scale.Factor) FactorResult {
	// 添加日志：记录领域层的 ScoringParams
	scoringParams := f.GetScoringParams()
	if scoringParams != nil {
		cntContentsJSON, _ := json.Marshal(scoringParams.GetCntOptionContents())
		logger.L(nil).Infow("toFactorResult: Domain ScoringParams",
			"factor_code", f.GetCode().String(),
			"scoring_strategy", f.GetScoringStrategy().String(),
			"cnt_option_contents", string(cntContentsJSON),
		)
	}
	
	// 转换计分参数为 map[string]interface{}（用于结果输出）
	scoringParamsMap := f.GetScoringParams().ToMap(f.GetScoringStrategy())
	
	// 添加日志：记录转换后的 map
	scoringParamsMapJSON, _ := json.Marshal(scoringParamsMap)
	logger.L(nil).Infow("toFactorResult: Application ScoringParams",
		"factor_code", f.GetCode().String(),
		"scoring_params", string(scoringParamsMapJSON),
	)
	
	result := FactorResult{
		Code:            f.GetCode().String(),
		Title:           f.GetTitle(),
		FactorType:      f.GetFactorType().String(),
		IsTotalScore:    f.IsTotalScore(),
		QuestionCodes:   make([]string, 0),
		ScoringStrategy: f.GetScoringStrategy().String(),
		ScoringParams:   scoringParamsMap,
		InterpretRules:  make([]InterpretRuleResult, 0),
	}

	// 转换题目编码列表
	for _, code := range f.GetQuestionCodes() {
		result.QuestionCodes = append(result.QuestionCodes, code.String())
	}

	// 转换解读规则列表
	for _, rule := range f.GetInterpretRules() {
		result.InterpretRules = append(result.InterpretRules, InterpretRuleResult{
			MinScore:   rule.GetScoreRange().Min(),
			MaxScore:   rule.GetScoreRange().Max(),
			RiskLevel:  rule.GetRiskLevel().String(),
			Conclusion: rule.GetConclusion(),
			Suggestion: rule.GetSuggestion(),
		})
	}

	return result
}

// toScaleListResult 将量表列表转换为结果对象
func toScaleListResult(items []*scale.MedicalScale, total int64) *ScaleListResult {
	result := &ScaleListResult{
		Items: make([]*ScaleResult, 0, len(items)),
		Total: total,
	}

	for _, item := range items {
		result.Items = append(result.Items, toScaleResult(item))
	}

	return result
}

// toSummaryListResult 将量表摘要列表转换为结果对象
func toSummaryListResult(items []*scale.ScaleSummary, total int64) *ScaleSummaryListResult {
	result := &ScaleSummaryListResult{
		Items: make([]*ScaleSummaryResult, 0, len(items)),
		Total: total,
	}

	for _, item := range items {
		result.Items = append(result.Items, &ScaleSummaryResult{
			Code:              item.Code,
			Title:             item.Title,
			Description:       item.Description,
			QuestionnaireCode: item.QuestionnaireCode,
			Status:            item.Status.String(),
		})
	}

	return result
}
