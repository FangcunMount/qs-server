package scale

import (
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

	po := &ScalePO{
		Code:                 domain.GetCode().String(),
		Title:                domain.GetTitle(),
		Description:          domain.GetDescription(),
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
func (m *ScaleMapper) ToDomain(po *ScalePO) *scale.MedicalScale {
	if po == nil {
		return nil
	}

	// 转换因子列表
	factors := m.mapFactorsToDomain(po.Factors)

	// 创建领域模型
	domain, err := scale.NewMedicalScale(
		meta.NewCode(po.Code),
		po.Title,
		scale.WithDescription(po.Description),
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
func (m *ScaleMapper) mapFactorsToDomain(factors []FactorPO) []*scale.Factor {
	if factors == nil {
		return []*scale.Factor{}
	}

	result := make([]*scale.Factor, 0, len(factors))
	for _, f := range factors {
		if factor := m.mapFactorToDomain(f); factor != nil {
			result = append(result, factor)
		}
	}
	return result
}

// mapFactorToDomain 将单个因子持久化对象转换为领域模型
func (m *ScaleMapper) mapFactorToDomain(po FactorPO) *scale.Factor {
	// 转换题目编码
	questionCodes := make([]meta.Code, 0, len(po.QuestionCodes))
	for _, qc := range po.QuestionCodes {
		questionCodes = append(questionCodes, meta.NewCode(qc))
	}

	// 转换解读规则
	interpretRules := m.mapInterpretRulesToDomain(po.InterpretRules)

	// 从 map[string]interface{} 恢复计分参数
	// 添加日志：记录 PO 层的 scoring_params
	scoringParamsJSON, _ := json.Marshal(po.ScoringParams)
	logger.L(nil).Infow("mapFactorToDomain: PO scoring_params",
		"factor_code", po.Code,
		"scoring_strategy", po.ScoringStrategy,
		"scoring_params", string(scoringParamsJSON),
		"scoring_params_type", getTypeName(po.ScoringParams),
	)

	scoringParams := scale.ScoringParamsFromMap(po.ScoringParams, scale.ScoringStrategyCode(po.ScoringStrategy))

	// 添加日志：记录转换后的 ScoringParams
	if scoringParams != nil {
		cntContentsJSON, _ := json.Marshal(scoringParams.GetCntOptionContents())
		logger.L(nil).Infow("mapFactorToDomain: Domain ScoringParams",
			"factor_code", po.Code,
			"cnt_option_contents", string(cntContentsJSON),
		)
	}

	// 创建因子
	factor, err := scale.NewFactor(
		scale.NewFactorCode(po.Code),
		po.Title,
		scale.WithFactorType(scale.FactorType(po.FactorType)),
		scale.WithIsTotalScore(po.IsTotalScore),
		scale.WithQuestionCodes(questionCodes),
		scale.WithScoringStrategy(scale.ScoringStrategyCode(po.ScoringStrategy)),
		scale.WithScoringParams(scoringParams),
		scale.WithInterpretRules(interpretRules),
	)
	if err != nil {
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
func (m *ScaleMapper) mapInterpretRulesToDomain(rules []InterpretRulePO) []scale.InterpretationRule {
	if rules == nil {
		return []scale.InterpretationRule{}
	}

	result := make([]scale.InterpretationRule, 0, len(rules))
	for _, r := range rules {
		rule := scale.NewInterpretationRule(
			scale.NewScoreRange(r.MinScore, r.MaxScore),
			scale.RiskLevel(r.RiskLevel),
			r.Conclusion,
			r.Suggestion,
		)
		result = append(result, rule)
	}
	return result
}
