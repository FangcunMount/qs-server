package scale

import (
	"context"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// ============= Result 定义 =============
// Results 用于应用服务层的输出结果

// ScaleResult 量表结果
type ScaleResult struct {
	Code                 string         // 量表编码
	Title                string         // 量表标题
	Description          string         // 量表描述
	Category             string         // 主类
	Stages               []string       // 阶段列表
	ApplicableAges       []string       // 使用年龄列表
	Reporters            []string       // 填报人列表
	Tags                 []string       // 标签列表
	QuestionnaireCode    string         // 关联的问卷编码
	QuestionnaireVersion string         // 关联的问卷版本
	Status               uint8          // 状态
	Factors              []FactorResult // 因子列表
	QRCodeURL            string         // 小程序码URL（仅已发布状态时返回）
	CreatedBy            string         // 创建人
	CreatedAt            time.Time      // 创建时间
	UpdatedBy            string         // 更新人
	UpdatedAt            time.Time      // 更新时间
}

// FactorResult 因子结果
type FactorResult struct {
	Code            string                 // 因子编码
	Title           string                 // 因子标题
	FactorType      string                 // 因子类型
	IsTotalScore    bool                   // 是否为总分因子
	IsShow          bool                   // 是否显示（用于报告中的维度展示）
	QuestionCodes   []string               // 关联的题目编码列表
	ScoringStrategy string                 // 计分策略
	ScoringParams   map[string]interface{} // 计分参数
	MaxScore        *float64               // 最大分
	RiskLevel       string                 // 因子级别的风险等级（从解读规则中提取，如果有多个规则则使用第一个规则的风险等级）
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
	Code              string    // 量表编码
	Title             string    // 量表标题
	Description       string    // 量表描述
	Category          string    // 主类
	Stages            []string  // 阶段列表
	ApplicableAges    []string  // 使用年龄列表
	Reporters         []string  // 填报人列表
	Tags              []string  // 标签列表
	QuestionnaireCode string    // 关联的问卷编码
	Status            uint8     // 状态
	CreatedBy         string    // 创建人
	CreatedAt         time.Time // 创建时间
	UpdatedBy         string    // 更新人
	UpdatedAt         time.Time // 更新时间
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

	// 转换标签列表
	tags := make([]string, 0, len(m.GetTags()))
	for _, tag := range m.GetTags() {
		tags = append(tags, tag.String())
	}

	// 转换填报人列表
	reporters := make([]string, 0, len(m.GetReporters()))
	for _, reporter := range m.GetReporters() {
		reporters = append(reporters, reporter.String())
	}

	// 转换阶段列表
	stages := make([]string, 0, len(m.GetStages()))
	for _, stage := range m.GetStages() {
		stages = append(stages, stage.String())
	}

	// 转换使用年龄列表
	applicableAges := make([]string, 0, len(m.GetApplicableAges()))
	for _, age := range m.GetApplicableAges() {
		applicableAges = append(applicableAges, age.String())
	}

	result := &ScaleResult{
		Code:                 m.GetCode().String(),
		Title:                m.GetTitle(),
		Description:          m.GetDescription(),
		Category:             m.GetCategory().String(),
		Stages:               stages,
		ApplicableAges:       applicableAges,
		Reporters:            reporters,
		Tags:                 tags,
		QuestionnaireCode:    m.GetQuestionnaireCode().String(),
		QuestionnaireVersion: m.GetQuestionnaireVersion(),
		Status:               m.GetStatus().Value(),
		Factors:              make([]FactorResult, 0),
		CreatedBy:            m.GetCreatedBy().String(),
		CreatedAt:            m.GetCreatedAt(),
		UpdatedBy:            m.GetUpdatedBy().String(),
		UpdatedAt:            m.GetUpdatedAt(),
	}

	// 转换因子列表
	for _, factor := range m.GetFactors() {
		result.Factors = append(result.Factors, toFactorResult(factor))
	}

	return result
}

func toScaleResultWithUsers(ctx context.Context, m *scale.MedicalScale, identitySvc *iam.IdentityService) *ScaleResult {
	if m == nil {
		return nil
	}

	userNames := iam.ResolveUserNames(ctx, identitySvc, []meta.ID{m.GetCreatedBy(), m.GetUpdatedBy()})
	result := toScaleResult(m)
	result.CreatedBy = iam.DisplayName(m.GetCreatedBy(), userNames)
	result.UpdatedBy = iam.DisplayName(m.GetUpdatedBy(), userNames)
	return result
}

// toFactorResult 将因子领域模型转换为结果对象
func toFactorResult(f *scale.Factor) FactorResult {
	// 转换计分参数为 map[string]interface{}（用于结果输出）
	scoringParamsMap := f.GetScoringParams().ToMap(f.GetScoringStrategy())

	result := FactorResult{
		Code:            f.GetCode().String(),
		Title:           f.GetTitle(),
		FactorType:      f.GetFactorType().String(),
		IsTotalScore:    f.IsTotalScore(),
		IsShow:          f.IsShow(),
		QuestionCodes:   make([]string, 0),
		ScoringStrategy: f.GetScoringStrategy().String(),
		ScoringParams:   scoringParamsMap,
		MaxScore:        f.GetMaxScore(),
		RiskLevel:       "", // 默认值，将从解读规则中提取
		InterpretRules:  make([]InterpretRuleResult, 0),
	}

	// 转换题目编码列表
	for _, code := range f.GetQuestionCodes() {
		result.QuestionCodes = append(result.QuestionCodes, code.String())
	}

	// 转换解读规则列表，并从第一个规则中提取风险等级作为因子级别的默认风险等级
	rules := f.GetInterpretRules()
	for i, rule := range rules {
		riskLevel := rule.GetRiskLevel().String()
		result.InterpretRules = append(result.InterpretRules, InterpretRuleResult{
			MinScore:   rule.GetScoreRange().Min(),
			MaxScore:   rule.GetScoreRange().Max(),
			RiskLevel:  riskLevel,
			Conclusion: rule.GetConclusion(),
			Suggestion: rule.GetSuggestion(),
		})
		// 使用第一个规则的风险等级作为因子级别的默认风险等级
		if i == 0 {
			result.RiskLevel = riskLevel
		}
	}

	// 如果没有解读规则，使用默认值 "none"
	if len(rules) == 0 {
		result.RiskLevel = "none"
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
func toSummaryListResult(ctx context.Context, items []*scale.MedicalScale, total int64, identitySvc *iam.IdentityService) *ScaleSummaryListResult {
	userNames := resolveSummaryUserNames(ctx, items, identitySvc)
	result := &ScaleSummaryListResult{
		Items: make([]*ScaleSummaryResult, 0, len(items)),
		Total: total,
	}

	for _, item := range items {
		// 转换标签列表
		tags := make([]string, 0, len(item.GetTags()))
		for _, tag := range item.GetTags() {
			tags = append(tags, tag.String())
		}

		// 转换填报人列表
		reporters := make([]string, 0, len(item.GetReporters()))
		for _, reporter := range item.GetReporters() {
			reporters = append(reporters, reporter.String())
		}

		// 转换阶段列表
		stages := make([]string, 0, len(item.GetStages()))
		for _, stage := range item.GetStages() {
			stages = append(stages, stage.String())
		}

		// 转换使用年龄列表
		applicableAges := make([]string, 0, len(item.GetApplicableAges()))
		for _, age := range item.GetApplicableAges() {
			applicableAges = append(applicableAges, age.String())
		}

		result.Items = append(result.Items, &ScaleSummaryResult{
			Code:              item.GetCode().String(),
			Title:             item.GetTitle(),
			Description:       item.GetDescription(),
			Category:          item.GetCategory().String(),
			Stages:            stages,
			ApplicableAges:    applicableAges,
			Reporters:         reporters,
			Tags:              tags,
			QuestionnaireCode: item.GetQuestionnaireCode().String(),
			Status:            item.GetStatus().Value(),
			CreatedBy:         iam.DisplayName(item.GetCreatedBy(), userNames),
			CreatedAt:         item.GetCreatedAt(),
			UpdatedBy:         iam.DisplayName(item.GetUpdatedBy(), userNames),
			UpdatedAt:         item.GetUpdatedAt(),
		})
	}

	return result
}

func resolveSummaryUserNames(ctx context.Context, items []*scale.MedicalScale, identitySvc *iam.IdentityService) map[string]string {
	userIDs := make([]meta.ID, 0, len(items)*2)
	for _, item := range items {
		if item == nil {
			continue
		}
		userIDs = append(userIDs, item.GetCreatedBy(), item.GetUpdatedBy())
	}
	return iam.ResolveUserNames(ctx, identitySvc, userIDs)
}
