package scale

import (
	"context"
	"encoding/json"
	"reflect"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"go.mongodb.org/mongo-driver/bson/primitive"
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
		if !stage.IsValid() {
			continue
		}
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
		Status:               domain.GetStatus().String(),
		Factors:              m.mapFactorsToPO(domain.GetFactors()),
	}
	po.CreatedAt = domain.GetCreatedAt()
	po.CreatedBy = domain.GetCreatedBy().Uint64()
	po.UpdatedAt = domain.GetUpdatedAt()
	po.UpdatedBy = domain.GetUpdatedBy().Uint64()

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

	return FactorPO{
		Code:            f.GetCode().String(),
		Title:           f.GetTitle(),
		FactorType:      f.GetFactorType().String(),
		IsTotalScore:    f.IsTotalScore(),
		IsShow:          f.IsShow(),
		QuestionCodes:   questionCodes,
		ScoringStrategy: f.GetScoringStrategy().String(),
		ScoringParams:   scoringParamsToStoredMap(f.GetScoringParams(), f.GetScoringStrategy()),
		MaxScore:        f.GetMaxScore(),
		InterpretRules:  m.mapInterpretRulesToPO(f.GetInterpretRules()),
	}
}

func scoringParamsToStoredMap(params *scale.ScoringParams, strategy scale.ScoringStrategyCode) map[string]interface{} {
	result := make(map[string]interface{})
	if params == nil {
		return result
	}
	switch strategy {
	case scale.ScoringStrategyCnt:
		contents := params.GetCntOptionContents()
		if len(contents) > 0 {
			result["cnt_option_contents"] = contents
		}
	case scale.ScoringStrategySum, scale.ScoringStrategyAvg:
		// These strategies currently do not require persisted params.
	default:
		// Unknown strategies are validated by the domain factor constructor.
	}
	return result
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
		stage := scale.NewStage(stageStr)
		if !stage.IsValid() {
			continue
		}
		stages = append(stages, stage)
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
		scale.WithCreatedBy(meta.FromUint64(po.CreatedBy)),
		scale.WithCreatedAt(po.CreatedAt),
		scale.WithUpdatedBy(meta.FromUint64(po.UpdatedBy)),
		scale.WithUpdatedAt(po.UpdatedAt),
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

	strategy := scale.ScoringStrategyCode(po.ScoringStrategy)
	scoringParams := m.mapScoringParamsToDomain(ctx, po.ScoringParams, strategy)

	// 添加日志：记录转换后的 ScoringParams
	if scoringParams != nil {
		cntContentsJSON, _ := json.Marshal(scoringParams.GetCntOptionContents())
		logger.L(ctx).Infow("mapFactorToDomain: Domain ScoringParams",
			"factor_code", po.Code,
			"cnt_option_contents", string(cntContentsJSON),
		)
	}

	// 验证：cnt 策略必须提供非空的 CntOptionContents
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
		scale.WithIsShow(po.IsShow),
		scale.WithQuestionCodes(questionCodes),
		scale.WithScoringStrategy(strategy),
		scale.WithScoringParams(scoringParams),
		scale.WithMaxScore(po.MaxScore),
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

func (m *ScaleMapper) mapScoringParamsToDomain(ctx context.Context, params map[string]interface{}, strategy scale.ScoringStrategyCode) *scale.ScoringParams {
	paramsJSON, _ := json.Marshal(params)
	logger.L(ctx).Infow("mapScoringParamsToDomain: input",
		"strategy", strategy,
		"params", string(paramsJSON),
		"params_type", getTypeName(params),
	)

	if len(params) == 0 {
		logger.L(ctx).Debugw("mapScoringParamsToDomain: params is nil or empty",
			"strategy", strategy,
		)
		return scale.NewScoringParams()
	}

	result := scale.NewScoringParams()
	switch strategy {
	case scale.ScoringStrategyCnt:
		contents, ok := params["cnt_option_contents"]
		if !ok || contents == nil {
			logger.L(ctx).Warnw("mapScoringParamsToDomain: cnt_option_contents not found",
				"strategy", strategy,
				"params_keys", getMapKeys(params),
			)
			break
		}

		result.WithCntOptionContents(stringSliceFromStoredArray(ctx, contents))
	case scale.ScoringStrategySum, scale.ScoringStrategyAvg:
		// These strategies currently do not require persisted params.
	default:
		// Unknown strategies are validated later by the domain factor constructor.
	}

	resultJSON, _ := json.Marshal(result.GetCntOptionContents())
	logger.L(ctx).Infow("mapScoringParamsToDomain: final result",
		"cnt_option_contents", string(resultJSON),
	)
	return result
}

func stringSliceFromStoredArray(ctx context.Context, value interface{}) []string {
	var values []interface{}
	switch v := value.(type) {
	case primitive.A:
		values = []interface{}(v)
	case []interface{}:
		values = v
	case []string:
		logger.L(ctx).Infow("mapScoringParamsToDomain: extracted cnt_option_contents (direct string array)",
			"count", len(v),
			"contents", v,
		)
		return v
	default:
		logger.L(ctx).Warnw("mapScoringParamsToDomain: cnt_option_contents is not array type",
			"contents_type", getTypeName(value),
		)
		return []string{}
	}

	result := make([]string, 0, len(values))
	for _, item := range values {
		str, ok := item.(string)
		if !ok {
			logger.L(ctx).Warnw("mapScoringParamsToDomain: array item is not string",
				"item_type", getTypeName(item),
				"item_value", item,
			)
			continue
		}
		result = append(result, str)
	}
	logger.L(ctx).Infow("mapScoringParamsToDomain: extracted cnt_option_contents",
		"count", len(result),
		"contents", result,
	)
	return result
}

// getMapKeys 获取 map 的键列表（用于日志记录）
func getMapKeys(m map[string]interface{}) []string {
	if m == nil {
		return []string{}
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
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
