package medicalscale

import (
	medicalscale "github.com/FangcunMount/qs-server/internal/apiserver/domain/medical-scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/medical-scale/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/medical-scale/factor/ability"
	base "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	"github.com/FangcunMount/qs-server/internal/pkg/calculation"
	"github.com/FangcunMount/qs-server/internal/pkg/interpretation"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// MedicalScaleMapper 医学量表映射器
type MedicalScaleMapper struct{}

// NewMedicalScaleMapper 创建医学量表映射器
func NewMedicalScaleMapper() *MedicalScaleMapper {
	return &MedicalScaleMapper{}
}

// ToPO 将领域模型转换为MongoDB持久化对象
func (m *MedicalScaleMapper) ToPO(bo *medicalscale.MedicalScale) *MedicalScalePO {
	if bo == nil {
		return nil
	}

	// 转换因子列表
	factors := make([]FactorPO, 0, len(bo.GetFactors()))
	for _, factor := range bo.GetFactors() {
		if po := m.mapFactorToPO(&factor); po != nil {
			factors = append(factors, *po)
		}
	}

	return &MedicalScalePO{
		BaseDocument: base.BaseDocument{
			ID: primitive.NewObjectID(),
		},
		Code:              bo.GetCode(),
		Title:             bo.GetTitle(),
		QuestionnaireCode: bo.GetQuestionnaireCode(),
		Factors:           factors,
	}
}

// ToBO 将MongoDB持久化对象转换为领域对象
func (m *MedicalScaleMapper) ToBO(po *MedicalScalePO) *medicalscale.MedicalScale {
	if po == nil {
		return nil
	}

	// 转换因子列表
	factors := make([]factor.Factor, 0, len(po.Factors))
	for _, factorPO := range po.Factors {
		if bo := m.mapFactorToBO(&factorPO); bo != nil {
			factors = append(factors, *bo)
		}
	}

	return medicalscale.NewMedicalScale(
		po.Code,
		po.Title,
		medicalscale.WithID(po.DomainID),
		medicalscale.WithQuestionnaireCode(po.QuestionnaireCode),
		medicalscale.WithFactors(factors),
	)
}

// mapFactorToPO 将因子领域对象转换为持久化对象
func (m *MedicalScaleMapper) mapFactorToPO(bo *factor.Factor) *FactorPO {
	if bo == nil {
		return nil
	}

	// 转换计算规则
	var calculationRule CalculationRulePO
	if bo.GetCalculationAbility() != nil && bo.GetCalculationAbility().GetCalculationRule() != nil {
		rule := bo.GetCalculationAbility().GetCalculationRule()
		calculationRule = CalculationRulePO{
			FormulaType: rule.GetFormula().String(),
			SourceCodes: rule.GetSourceCodes(),
		}
	}

	// 转换解读规则
	var interpretRules []InterpretRulePO
	if bo.GetInterpretationAbility() != nil {
		rules := bo.GetInterpretationAbility().GetInterpretationRules()
		interpretRules = make([]InterpretRulePO, len(rules))
		for i, rule := range rules {
			interpretRules[i] = InterpretRulePO{
				ScoreRange: ScoreRangePO{
					MinScore: rule.GetScoreRange().MinScore(),
					MaxScore: rule.GetScoreRange().MaxScore(),
				},
				Content: rule.GetContent(),
			}
		}
	}

	return &FactorPO{
		Code:            bo.GetCode(),
		Title:           bo.GetTitle(),
		FactorType:      bo.GetFactorType().String(),
		CalculationRule: calculationRule,
		InterpretRules:  interpretRules,
	}
}

// mapFactorToBO 将因子持久化对象转换为领域对象
func (m *MedicalScaleMapper) mapFactorToBO(po *FactorPO) *factor.Factor {
	if po == nil {
		return nil
	}

	// 转换计算规则
	var calculationAbility *ability.CalculationAbility
	if po.CalculationRule.FormulaType != "" {
		rule := calculation.NewCalculationRule(
			calculation.FormulaType(po.CalculationRule.FormulaType),
			po.CalculationRule.SourceCodes,
		)
		calculationAbility = &ability.CalculationAbility{}
		calculationAbility.SetCalculationRule(rule)
	}

	// 转换解读规则
	var interpretationAbility *ability.InterpretationAbility
	if len(po.InterpretRules) > 0 {
		rules := make([]interpretation.InterpretRule, len(po.InterpretRules))
		for i, rulePO := range po.InterpretRules {
			rules[i] = interpretation.NewInterpretRule(
				interpretation.NewScoreRange(
					rulePO.ScoreRange.MinScore,
					rulePO.ScoreRange.MaxScore,
				),
				rulePO.Content,
			)
		}
		interpretationAbility = &ability.InterpretationAbility{}
		interpretationAbility.SetInterpretationRules(rules)
	}

	result := factor.NewFactor(
		po.Code,
		po.Title,
		factor.FactorType(po.FactorType),
		factor.WithCalculation(calculationAbility),
		factor.WithInterpretation(interpretationAbility),
	)

	return &result
}
