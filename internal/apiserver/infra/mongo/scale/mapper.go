package scale

import (
	"context"
	"encoding/json"
	"reflect"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// ScaleMapper 量表映射器
type ScaleMapper struct{}

// NewScaleMapper 创建量表映射器
func NewScaleMapper() *ScaleMapper {
	return &ScaleMapper{}
}

// ToPO 将领域模型转换为持久化对象
func (m *ScaleMapper) ToPO(domain *scale.MedicalScale) *ScalePO {
	if domain == nil {
		return nil
	}

	// 转换标签列表
	tags := make([]string, 0, len(domain.GetTags()))
	for _, tag := range domain.GetTags() {
		tags = append(tags, tag.String())
	}

	// 转换填报人列表
	reporters := make([]string, 0, len(domain.GetReporters()))
	for _, reporter := range domain.GetReporters() {
		reporters = append(reporters, reporter.String())
	}

	// 转换阶段列表
	stages := make([]string, 0, len(domain.GetStages()))
	for _, stage := range domain.GetStages() {
		stages = append(stages, stage.String())
	}

	// 转换使用年龄列表
	applicableAges := make([]string, 0, len(domain.GetApplicableAges()))
	for _, age := range domain.GetApplicableAges() {
		applicableAges = append(applicableAges, age.String())
	}

	po := &ScalePO{
		Code:                 domain.GetCode().String(),
		Title:                domain.GetTitle(),
		Description:          domain.GetDescription(),
		Category:             domain.GetCategory().String(),
		Stages:               stages,
		ApplicableAges:       applicableAges,
		Reporters:            reporters,
		Tags:                 tags,
		QuestionnaireCode:    domain.GetQuestionnaireCode().String(),
		QuestionnaireVersion: domain.GetQuestionnaireVersion(),
		Status:               domain.GetStatus().Value(),
		Factors:              m.mapFactorsToPO(domain.GetFactors()),
	}

	return po
}

// mapFactorsToPO 将因子列表转换为持久化对象
func (m *ScaleMapper) mapFactorsToPO(factors []*scale.Factor) []FactorPO {
	if factors == nil {
		return []FactorPO{}
	}

	result := make([]FactorPO, 0, len(factors))
	for _, f := range factors {
		result = append(result, m.mapFactorToPO(f))
	}
	return result
}

// mapFactorToPO 将单个因子转换为持久化对象
func (m *ScaleMapper) mapFactorToPO(f *scale.Factor) FactorPO {
	// 转换题目编码
	questionCodes := make([]string, 0, len(f.GetQuestionCodes()))
	for _, qc := range f.GetQuestionCodes() {
		questionCodes = append(questionCodes, qc.String())
	}

	// 转换计分参数为 map[string]interface{}（用于持久化）
	scoringParamsMap := f.GetScoringParams().ToMap(f.GetScoringStrategy())

	return FactorPO{
		Code:            f.GetCode().String(),
		Title:           f.GetTitle(),
		FactorType:      f.GetFactorType().String(),
		IsTotalScore:    f.IsTotalScore(),
		QuestionCodes:   questionCodes,
		ScoringStrategy: f.GetScoringStrategy().String(),
		ScoringParams:   scoringParamsMap,
		InterpretRules:  m.mapInterpretRulesToPO(f.GetInterpretRules()),
	}
}

// mapInterpretRulesToPO 将解读规则列表转换为持久化对象
func (m *ScaleMapper) mapInterpretRulesToPO(rules []scale.InterpretationRule) []InterpretRulePO {
	if rules == nil {
		return []InterpretRulePO{}
	}

	result := make([]InterpretRulePO, 0, len(rules))
	for _, r := range rules {
		result = append(result, InterpretRulePO{
			MinScore:   r.GetScoreRange().Min(),
			MaxScore:   r.GetScoreRange().Max(),
			RiskLevel:  r.GetRiskLevel().String(),
			Conclusion: r.GetConclusion(),
			Suggestion: r.GetSuggestion(),
		})
	}
	return result
}

// ToDomain 将持久化对象转换为领域模型
func (m *ScaleMapper) ToDomain(ctx context.Context, po *ScalePO) *scale.MedicalScale {
	if po == nil {
		return nil
	}

	// 转换因子列表
	factors := m.mapFactorsToDomain(ctx, po.Factors)

	// 转换标签列表
	tags := make([]scale.Tag, 0, len(po.Tags))
	for _, tagStr := range po.Tags {
		tags = append(tags, scale.NewTag(tagStr))
	}

	// 转换填报人列表
	reporters := make([]scale.Reporter, 0, len(po.Reporters))
	for _, reporterStr := range po.Reporters {
		reporters = append(reporters, scale.NewReporter(reporterStr))
	}

	// 转换阶段列表
	stages := make([]scale.Stage, 0, len(po.Stages))
	for _, stageStr := range po.Stages {
		stages = append(stages, scale.NewStage(stageStr))
	}

	// 转换使用年龄列表
	applicableAges := make([]scale.ApplicableAge, 0, len(po.ApplicableAges))
	for _, ageStr := range po.ApplicableAges {
		applicableAges = append(applicableAges, scale.NewApplicableAge(ageStr))
	}

	// 创建领域模型
	domain, err := scale.NewMedicalScale(
		meta.NewCode(po.Code),
		po.Title,
		scale.WithDescription(po.Description),
		scale.WithCategory(scale.NewCategory(po.Category)),
		scale.WithStages(stages),
		scale.WithApplicableAges(applicableAges),
		scale.WithReporters(reporters),
		scale.WithTags(tags),
		scale.WithQuestionnaire(meta.NewCode(po.QuestionnaireCode), po.QuestionnaireVersion),
		scale.WithStatus(scale.Status(po.Status)),
		scale.WithFactors(factors),
	)
	if err != nil {
		// 如果创建失败，返回 nil（理论上不应该发生，因为 PO 数据应该是有效的）
		return nil
	}

	return domain
}

// mapFactorsToDomain 将因子持久化对象列表转换为领域模型
func (m *ScaleMapper) mapFactorsToDomain(ctx context.Context, factors []FactorPO) []*scale.Factor {
	if factors == nil {
		return []*scale.Factor{}
	}

	result := make([]*scale.Factor, 0, len(factors))
	for _, f := range factors {
		if factor := m.mapFactorToDomain(ctx, f); factor != nil {
			result = append(result, factor)
		}
	}
	return result
}

// mapFactorToDomain 将单个因子持久化对象转换为领域模型
func (m *ScaleMapper) mapFactorToDomain(ctx context.Context, po FactorPO) *scale.Factor {
	// 转换题目编码
	questionCodes := make([]meta.Code, 0, len(po.QuestionCodes))
	for _, qc := range po.QuestionCodes {
		questionCodes = append(questionCodes, meta.NewCode(qc))
	}

	// 转换解读规则
	interpretRules := m.mapInterpretRulesToDomain(ctx, po.InterpretRules)

	// 从 map[string]interface{} 恢复计分参数
	// 添加日志：记录 PO 层的 scoring_params
	scoringParamsJSON, _ := json.Marshal(po.ScoringParams)
	logger.L(ctx).Infow("mapFactorToDomain: PO scoring_params",
		"factor_code", po.Code,
		"scoring_strategy", po.ScoringStrategy,
		"scoring_params", string(scoringParamsJSON),
		"scoring_params_type", getTypeName(po.ScoringParams),
	)

	scoringParams := scale.ScoringParamsFromMap(ctx, po.ScoringParams, scale.ScoringStrategyCode(po.ScoringStrategy))

	// 添加日志：记录转换后的 ScoringParams
	if scoringParams != nil {
		cntContentsJSON, _ := json.Marshal(scoringParams.GetCntOptionContents())
		logger.L(ctx).Infow("mapFactorToDomain: Domain ScoringParams",
			"factor_code", po.Code,
			"cnt_option_contents", string(cntContentsJSON),
		)
	}

	// 验证：cnt 策略必须提供非空的 CntOptionContents
	strategy := scale.ScoringStrategyCode(po.ScoringStrategy)
	if strategy == scale.ScoringStrategyCnt {
		if scoringParams == nil || len(scoringParams.GetCntOptionContents()) == 0 {
			logger.L(ctx).Warnw("mapFactorToDomain: cnt strategy requires non-empty cnt_option_contents, skipping factor",
				"factor_code", po.Code,
				"scoring_params", po.ScoringParams,
			)
			// 返回 nil，让上层知道转换失败
			// 注意：这里不直接报错，因为可能是历史数据问题，让上层决定如何处理
			return nil
		}
	}

	// 创建因子
	factor, err := scale.NewFactor(
		scale.NewFactorCode(po.Code),
		po.Title,
		scale.WithFactorType(scale.FactorType(po.FactorType)),
		scale.WithIsTotalScore(po.IsTotalScore),
		scale.WithQuestionCodes(questionCodes),
		scale.WithScoringStrategy(strategy),
		scale.WithScoringParams(scoringParams),
		scale.WithInterpretRules(interpretRules),
	)
	if err != nil {
		logger.L(ctx).Errorw("mapFactorToDomain: failed to create factor",
			"factor_code", po.Code,
			"error", err.Error(),
		)
		return nil
	}

	return factor
}

// getTypeName 获取类型的字符串表示
func getTypeName(v interface{}) string {
	if v == nil {
		return "nil"
	}
	return reflect.TypeOf(v).String()
}

// mapInterpretRulesToDomain 将解读规则持久化对象列表转换为领域模型
func (m *ScaleMapper) mapInterpretRulesToDomain(ctx context.Context, rules []InterpretRulePO) []scale.InterpretationRule {
	if rules == nil {
		return []scale.InterpretationRule{}
	}

	result := make([]scale.InterpretationRule, 0, len(rules))
	for i, r := range rules {
		// 创建分数区间
		scoreRange := scale.NewScoreRange(r.MinScore, r.MaxScore)

		// 规范化风险等级（兼容旧数据）
		riskLevel := normalizeRiskLevel(r.RiskLevel)

		// 验证并记录日志
		if !scoreRange.IsValid() {
			logger.L(ctx).Warnw("mapInterpretRulesToDomain: invalid score range",
				"rule_index", i,
				"min_score", r.MinScore,
				"max_score", r.MaxScore,
				"risk_level", r.RiskLevel,
			)
		}
		if !riskLevel.IsValid() {
			logger.L(ctx).Warnw("mapInterpretRulesToDomain: invalid risk level after normalization",
				"rule_index", i,
				"original_risk_level", r.RiskLevel,
				"normalized_risk_level", riskLevel,
				"min_score", r.MinScore,
				"max_score", r.MaxScore,
			)
		}

		rule := scale.NewInterpretationRule(
			scoreRange,
			riskLevel,
			r.Conclusion,
			r.Suggestion,
		)
		result = append(result, rule)
	}
	return result
}

// normalizeRiskLevel 规范化风险等级字符串（兼容旧数据）
// 将旧的风险等级值映射到新的有效值
func normalizeRiskLevel(raw string) scale.RiskLevel {
	switch raw {
	case "none", "正常", "无风险", "normal":
		return scale.RiskLevelNone
	case "low", "轻度", "低风险":
		return scale.RiskLevelLow
	case "medium", "中度", "中风险":
		return scale.RiskLevelMedium
	case "high", "重度", "高风险":
		return scale.RiskLevelHigh
	case "severe", "严重", "极高风险":
		return scale.RiskLevelSevere
	default:
		// 如果无法映射，返回原始值（让验证层处理）
		return scale.RiskLevel(raw)
	}
}
