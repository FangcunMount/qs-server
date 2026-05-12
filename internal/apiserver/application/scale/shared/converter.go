package shared

import (
	"context"
	"time"

	domainScale "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	iambridge "github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/scalereadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// ============= Result 定义 =============
// Results 用于应用服务层的输出结果

// ScaleResult 量表结果
type ScaleResult struct {
	Code                 string         // 量表编码
	ScaleVersion         string         // 量表解释模型版本
	Title                string         // 量表标题
	Description          string         // 量表描述
	Category             string         // 主类
	Stages               []string       // 阶段列表
	ApplicableAges       []string       // 使用年龄列表
	Reporters            []string       // 填报人列表
	Tags                 []string       // 标签列表
	QuestionnaireCode    string         // 关联的问卷编码
	QuestionnaireVersion string         // 关联的问卷版本
	QuestionCount        int32          // 问卷题目数量（摘要/详情显示）
	Status               string         // 状态
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
	ScaleVersion      string    // 量表解释模型版本
	Title             string    // 量表标题
	Description       string    // 量表描述
	Category          string    // 主类
	Stages            []string  // 阶段列表
	ApplicableAges    []string  // 使用年龄列表
	Reporters         []string  // 填报人列表
	Tags              []string  // 标签列表
	QuestionnaireCode string    // 关联的问卷编码
	QuestionCount     int32     // 问卷题目数量
	Status            string    // 状态
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

// HotScaleSummaryResult 热门量表摘要结果。
type HotScaleSummaryResult struct {
	ScaleSummaryResult
	Rank            int   // 排名，从 1 开始
	SubmissionCount int64 // 统计窗口内提交数
	HeatScore       int64 // 当前热度分，现阶段等于提交数
}

// HotScaleListResult 热门量表列表结果。
type HotScaleListResult struct {
	Items      []*HotScaleSummaryResult
	Total      int64
	Limit      int
	WindowDays int
}

// ============= Converter 转换器 =============

// ToScaleResult 将领域模型转换为结果对象
func ToScaleResult(m *domainScale.MedicalScale) *ScaleResult {
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
		ScaleVersion:         m.GetScaleVersion(),
		Title:                m.GetTitle(),
		Description:          m.GetDescription(),
		Category:             m.GetCategory().String(),
		Stages:               stages,
		ApplicableAges:       applicableAges,
		Reporters:            reporters,
		Tags:                 tags,
		QuestionnaireCode:    m.GetQuestionnaireCode().String(),
		QuestionnaireVersion: m.GetQuestionnaireVersion(),
		// QuestionCount 由问卷预存字段提供，若领域未持有则在上层补齐
		Status:    m.GetStatus().String(),
		Factors:   make([]FactorResult, 0),
		CreatedBy: m.GetCreatedBy().String(),
		CreatedAt: m.GetCreatedAt(),
		UpdatedBy: m.GetUpdatedBy().String(),
		UpdatedAt: m.GetUpdatedAt(),
	}

	for _, snapshot := range m.FactorSnapshots() {
		result.Factors = append(result.Factors, ToFactorResult(snapshot))
	}

	return result
}

func ToScaleResultWithUsers(ctx context.Context, m *domainScale.MedicalScale, identitySvc iambridge.IdentityResolver) *ScaleResult {
	if m == nil {
		return nil
	}

	userNames := ResolveIdentityNames(ctx, identitySvc, []meta.ID{m.GetCreatedBy(), m.GetUpdatedBy()})
	result := ToScaleResult(m)
	result.CreatedBy = DisplayIdentityName(m.GetCreatedBy(), userNames)
	result.UpdatedBy = DisplayIdentityName(m.GetUpdatedBy(), userNames)
	return result
}

// ToFactorResult 将因子只读快照转换为结果对象。
func ToFactorResult(snapshot domainScale.FactorSnapshot) FactorResult {
	result := FactorResult{
		Code:            snapshot.Code.String(),
		Title:           snapshot.Title,
		FactorType:      snapshot.FactorType.String(),
		IsTotalScore:    snapshot.IsTotalScore,
		IsShow:          snapshot.IsShow,
		QuestionCodes:   make([]string, 0, len(snapshot.QuestionCodes)),
		ScoringStrategy: snapshot.ScoringStrategy.String(),
		ScoringParams:   scoringParamsResultMap(snapshot.ScoringParams, snapshot.ScoringStrategy),
		MaxScore:        snapshot.MaxScore,
		RiskLevel:       "",
		InterpretRules:  make([]InterpretRuleResult, 0, len(snapshot.InterpretRules)),
	}

	for _, code := range snapshot.QuestionCodes {
		result.QuestionCodes = append(result.QuestionCodes, code.String())
	}

	for i, rule := range snapshot.InterpretRules {
		riskLevel := rule.GetRiskLevel().String()
		result.InterpretRules = append(result.InterpretRules, InterpretRuleResult{
			MinScore:   rule.GetScoreRange().Min(),
			MaxScore:   rule.GetScoreRange().Max(),
			RiskLevel:  riskLevel,
			Conclusion: rule.GetConclusion(),
			Suggestion: rule.GetSuggestion(),
		})
		if i == 0 {
			result.RiskLevel = riskLevel
		}
	}

	if len(snapshot.InterpretRules) == 0 {
		result.RiskLevel = "none"
	}

	return result
}

func scoringParamsResultMap(params *domainScale.ScoringParams, strategy domainScale.ScoringStrategyCode) map[string]interface{} {
	result := make(map[string]interface{})
	if params == nil {
		return result
	}
	switch strategy {
	case domainScale.ScoringStrategyCnt:
		contents := params.GetCntOptionContents()
		if len(contents) > 0 {
			result["cnt_option_contents"] = contents
		}
	case domainScale.ScoringStrategySum, domainScale.ScoringStrategyAvg:
		// These strategies currently do not expose additional params.
	default:
		// Keep unknown strategies empty; strategy validation belongs to the domain.
	}
	return result
}

// ToSummaryListResult 将量表摘要列表转换为结果对象
func ToSummaryListResult(ctx context.Context, items []*domainScale.MedicalScale, total int64, identitySvc iambridge.IdentityResolver) *ScaleSummaryListResult {
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
			ScaleVersion:      item.GetScaleVersion(),
			Title:             item.GetTitle(),
			Description:       item.GetDescription(),
			Category:          item.GetCategory().String(),
			Stages:            stages,
			ApplicableAges:    applicableAges,
			Reporters:         reporters,
			Tags:              tags,
			QuestionnaireCode: item.GetQuestionnaireCode().String(),
			Status:            item.GetStatus().String(),
			CreatedBy:         DisplayIdentityName(item.GetCreatedBy(), userNames),
			CreatedAt:         item.GetCreatedAt(),
			UpdatedBy:         DisplayIdentityName(item.GetUpdatedBy(), userNames),
			UpdatedAt:         item.GetUpdatedAt(),
		})
	}

	return result
}

func ToSummaryRowsResult(ctx context.Context, items []scalereadmodel.ScaleSummaryRow, total int64, identitySvc iambridge.IdentityResolver) *ScaleSummaryListResult {
	userNames := resolveScaleRowUserNames(ctx, items, identitySvc)
	result := &ScaleSummaryListResult{
		Items: make([]*ScaleSummaryResult, 0, len(items)),
		Total: total,
	}

	for _, item := range items {
		result.Items = append(result.Items, &ScaleSummaryResult{
			Code:              item.Code,
			ScaleVersion:      item.ScaleVersion,
			Title:             item.Title,
			Description:       item.Description,
			Category:          item.Category,
			Stages:            append([]string(nil), item.Stages...),
			ApplicableAges:    append([]string(nil), item.ApplicableAges...),
			Reporters:         append([]string(nil), item.Reporters...),
			Tags:              append([]string(nil), item.Tags...),
			QuestionnaireCode: item.QuestionnaireCode,
			QuestionCount:     item.QuestionCount,
			Status:            item.Status,
			CreatedBy:         DisplayIdentityName(item.CreatedBy, userNames),
			CreatedAt:         item.CreatedAt,
			UpdatedBy:         DisplayIdentityName(item.UpdatedBy, userNames),
			UpdatedAt:         item.UpdatedAt,
		})
	}

	return result
}

func ToHotScaleListResult(ctx context.Context, items []domainScale.HotScaleSummary, limit, windowDays int, identitySvc iambridge.IdentityResolver) *HotScaleListResult {
	hotItems := make([]domainScale.HotScaleSummary, 0, len(items))
	scales := make([]*domainScale.MedicalScale, 0, len(items))
	for _, item := range items {
		if item.Scale != nil {
			hotItems = append(hotItems, item)
			scales = append(scales, item.Scale)
		}
	}
	summary := ToSummaryListResult(ctx, scales, int64(len(scales)), identitySvc)

	result := &HotScaleListResult{
		Items:      make([]*HotScaleSummaryResult, 0, len(summary.Items)),
		Total:      int64(len(summary.Items)),
		Limit:      limit,
		WindowDays: windowDays,
	}
	for i, summaryItem := range summary.Items {
		submissionCount := hotItems[i].SubmissionCount
		result.Items = append(result.Items, &HotScaleSummaryResult{
			ScaleSummaryResult: *summaryItem,
			Rank:               i + 1,
			SubmissionCount:    submissionCount,
			HeatScore:          submissionCount,
		})
	}
	return result
}

func resolveSummaryUserNames(ctx context.Context, items []*domainScale.MedicalScale, identitySvc iambridge.IdentityResolver) map[string]string {
	userIDs := make([]meta.ID, 0, len(items)*2)
	for _, item := range items {
		if item == nil {
			continue
		}
		userIDs = append(userIDs, item.GetCreatedBy(), item.GetUpdatedBy())
	}
	return ResolveIdentityNames(ctx, identitySvc, userIDs)
}

func resolveScaleRowUserNames(ctx context.Context, items []scalereadmodel.ScaleSummaryRow, identitySvc iambridge.IdentityResolver) map[string]string {
	userIDs := make([]meta.ID, 0, len(items)*2)
	for _, item := range items {
		userIDs = append(userIDs, item.CreatedBy, item.UpdatedBy)
	}
	return ResolveIdentityNames(ctx, identitySvc, userIDs)
}

func ResolveIdentityNames(ctx context.Context, identitySvc iambridge.IdentityResolver, ids []meta.ID) map[string]string {
	if identitySvc == nil || !identitySvc.IsEnabled() {
		return nil
	}
	return identitySvc.ResolveUserNames(ctx, ids)
}

func DisplayIdentityName(id meta.ID, userNames map[string]string) string {
	if id.IsZero() {
		return ""
	}
	if userNames != nil {
		if name, ok := userNames[id.String()]; ok && name != "" {
			return name
		}
	}
	return id.String()
}
