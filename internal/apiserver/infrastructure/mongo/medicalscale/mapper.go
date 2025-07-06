package medicalscale

import (
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/medicalscale"
	mongoBase "github.com/yshujie/questionnaire-scale/internal/apiserver/infrastructure/mongo"
)

// MedicalScaleMapper 医学量表映射器
type MedicalScaleMapper struct{}

// NewMedicalScaleMapper 创建医学量表映射器
func NewMedicalScaleMapper() *MedicalScaleMapper {
	return &MedicalScaleMapper{}
}

// ToPO 将领域模型转换为MongoDB持久化对象
func (m *MedicalScaleMapper) ToPO(bo *medicalscale.MedicalScale) *MedicalScalePO {
	po := &MedicalScalePO{
		Code:                 bo.Code(),
		Title:                bo.Title(),
		QuestionnaireCode:    bo.QuestionnaireCode(),
		QuestionnaireVersion: bo.QuestionnaireVersion(),
		Factors:              m.mapFactorsToPO(bo.Factors()),
	}

	// 设置基础文档字段
	po.BaseDocument.SetCreatedAt(bo.CreatedAt())
	po.BaseDocument.SetUpdatedAt(bo.UpdatedAt())

	// 如果有ID，则设置
	if bo.ID().Value() != 0 {
		// 将领域ID转换为ObjectID
		if objectID, err := mongoBase.Uint64ToObjectID(bo.ID().Value()); err == nil {
			po.BaseDocument.ID = objectID
		}
	}

	return po
}

// mapFactorsToPO 将因子领域对象转换为持久化对象
func (m *MedicalScaleMapper) mapFactorsToPO(factors []medicalscale.Factor) []FactorPO {
	if factors == nil {
		return []FactorPO{}
	}

	var factorsPO []FactorPO
	for _, factor := range factors {
		factorPO := FactorPO{
			Code:            factor.Code(),
			Title:           factor.Title(),
			IsTotalScore:    factor.IsTotalScore(),
			FactorType:      factor.Type().String(),
			CalculationRule: m.mapCalculationRuleToPO(factor.CalculationRule()),
			InterpretRules:  m.mapInterpretRulesToPO(factor.InterpretRules()),
		}
		factorsPO = append(factorsPO, factorPO)
	}

	return factorsPO
}

// mapCalculationRuleToPO 将计算规则转换为持久化对象
func (m *MedicalScaleMapper) mapCalculationRuleToPO(rule medicalscale.CalculationRule) CalculationRulePO {
	return CalculationRulePO{
		FormulaType: rule.FormulaType().String(),
		SourceCodes: rule.SourceCodes(),
	}
}

// mapInterpretRulesToPO 将解读规则转换为持久化对象
func (m *MedicalScaleMapper) mapInterpretRulesToPO(rules []medicalscale.InterpretRule) []InterpretRulePO {
	if rules == nil {
		return []InterpretRulePO{}
	}

	var rulesPO []InterpretRulePO
	for _, rule := range rules {
		rulePO := InterpretRulePO{
			ScoreRange: ScoreRangePO{
				MinScore: rule.ScoreRange().MinScore(),
				MaxScore: rule.ScoreRange().MaxScore(),
			},
			Content: rule.Content(),
		}
		rulesPO = append(rulesPO, rulePO)
	}

	return rulesPO
}

// ToBO 将MongoDB持久化对象转换为业务对象
func (m *MedicalScaleMapper) ToBO(po *MedicalScalePO) *medicalscale.MedicalScale {
	// 转换ID
	var id medicalscale.MedicalScaleID
	if !po.BaseDocument.ID.IsZero() {
		domainID := mongoBase.ObjectIDToUint64(po.BaseDocument.ID)
		id = medicalscale.NewMedicalScaleID(domainID)
	}

	// 转换因子
	factors := m.mapFactorsToBO(po.Factors)

	// 创建医学量表对象
	ms := medicalscale.NewMedicalScale(
		id,
		po.Code,
		po.Title,
		po.QuestionnaireCode,
		po.QuestionnaireVersion,
		factors,
	)

	return ms
}

// mapFactorsToBO 将因子持久化对象转换为领域对象
func (m *MedicalScaleMapper) mapFactorsToBO(factorsPO []FactorPO) []medicalscale.Factor {
	if factorsPO == nil {
		return []medicalscale.Factor{}
	}

	var factors []medicalscale.Factor
	for _, factorPO := range factorsPO {
		factor := medicalscale.NewFactor(
			factorPO.Code,
			factorPO.Title,
			factorPO.IsTotalScore,
			medicalscale.FactorType(factorPO.FactorType),
			m.mapCalculationRuleToBO(factorPO.CalculationRule),
			m.mapInterpretRulesToBO(factorPO.InterpretRules),
		)
		factors = append(factors, factor)
	}

	return factors
}

// mapCalculationRuleToBO 将计算规则持久化对象转换为领域对象
func (m *MedicalScaleMapper) mapCalculationRuleToBO(rulePO CalculationRulePO) medicalscale.CalculationRule {
	return medicalscale.NewCalculationRule(
		medicalscale.FormulaType(rulePO.FormulaType),
		rulePO.SourceCodes,
	)
}

// mapInterpretRulesToBO 将解读规则持久化对象转换为领域对象
func (m *MedicalScaleMapper) mapInterpretRulesToBO(rulesPO []InterpretRulePO) []medicalscale.InterpretRule {
	if rulesPO == nil {
		return []medicalscale.InterpretRule{}
	}

	var rules []medicalscale.InterpretRule
	for _, rulePO := range rulesPO {
		scoreRange := medicalscale.NewScoreRange(
			rulePO.ScoreRange.MinScore,
			rulePO.ScoreRange.MaxScore,
		)
		rule := medicalscale.NewInterpretRule(scoreRange, rulePO.Content)
		rules = append(rules, rule)
	}

	return rules
}
