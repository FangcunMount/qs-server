package medicalscale

import (
	medicalscale "github.com/yshujie/questionnaire-scale/internal/apiserver/domain/medical-scale"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/medical-scale/factor"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/medical-scale/factor/ability"
	mongoBase "github.com/yshujie/questionnaire-scale/internal/apiserver/infrastructure/mongo"
	"github.com/yshujie/questionnaire-scale/internal/pkg/calculation"
	"github.com/yshujie/questionnaire-scale/internal/pkg/interpretation"
	v1 "github.com/yshujie/questionnaire-scale/pkg/meta/v1"
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
		Code:              bo.GetCode(),
		Title:             bo.GetTitle(),
		QuestionnaireCode: bo.GetQuestionnaireCode(),
		Factors:           m.mapFactorsToPO(bo.GetFactors()),
	}

	if bo.GetID().Value() != 0 {
		objectID, _ := mongoBase.Uint64ToObjectID(bo.GetID().Value())
		po.BaseDocument.ID = objectID
	}

	return po
}

// mapFactorsToPO 将因子领域对象转换为持久化对象
func (m *MedicalScaleMapper) mapFactorsToPO(factors []factor.Factor) []FactorPO {
	if factors == nil {
		return []FactorPO{}
	}

	var factorsPO []FactorPO
	for _, f := range factors {
		factorPO := FactorPO{
			Code:       f.GetCode(),
			Title:      f.GetTitle(),
			FactorType: f.GetFactorType().String(),
		}

		// 处理计算能力
		if calcAbility := f.GetCalculationAbility(); calcAbility != nil {
			if calcRule := calcAbility.GetCalculationRule(); calcRule != nil {
				factorPO.CalculationRule = CalculationRulePO{
					FormulaType: calcRule.GetFormula().String(),
					SourceCodes: calcRule.GetSourceCodes(),
				}
			}
		}

		// 处理解读能力
		if interpretAbility := f.GetInterpretationAbility(); interpretAbility != nil {
			if interpretRule := interpretAbility.GetInterpretationRule(); interpretRule != nil {
				factorPO.InterpretRules = []InterpretRulePO{
					{
						ScoreRange: ScoreRangePO{
							MinScore: interpretRule.GetScoreRange().MinScore(),
							MaxScore: interpretRule.GetScoreRange().MaxScore(),
						},
						Content: interpretRule.GetContent(),
					},
				}
			}
		}

		factorsPO = append(factorsPO, factorPO)
	}

	return factorsPO
}

// ToBO 将MongoDB持久化对象转换为业务对象
func (m *MedicalScaleMapper) ToBO(po *MedicalScalePO) *medicalscale.MedicalScale {
	// 转换因子
	factors := m.mapFactorsToBO(po.Factors)

	// 创建医学量表选项
	opts := []medicalscale.MedicalScaleOption{
		medicalscale.WithCode(po.Code),
		medicalscale.WithTitle(po.Title),
		medicalscale.WithQuestionnaireCode(po.QuestionnaireCode),
		medicalscale.WithFactors(factors),
	}

	// 如果有ID，添加ID选项
	if !po.BaseDocument.ID.IsZero() {
		domainID := mongoBase.ObjectIDToUint64(po.BaseDocument.ID)
		opts = append(opts, medicalscale.WithID(v1.NewID(domainID)))
	}

	// 创建医学量表对象
	ms := medicalscale.NewMedicalScale(po.Code, po.Title, opts...)

	return ms
}

// mapFactorsToBO 将因子持久化对象转换为领域对象
func (m *MedicalScaleMapper) mapFactorsToBO(factorsPO []FactorPO) []factor.Factor {
	if factorsPO == nil {
		return []factor.Factor{}
	}

	var factors []factor.Factor
	for _, factorPO := range factorsPO {
		var opts []factor.FactorOption

		// 处理计算规则
		if factorPO.CalculationRule.FormulaType != "" {
			calculationRule := calculation.NewCalculationRule(
				calculation.FormulaType(factorPO.CalculationRule.FormulaType),
				factorPO.CalculationRule.SourceCodes,
			)
			calculationAbility := &ability.CalculationAbility{}
			calculationAbility.SetCalculationRule(calculationRule)
			opts = append(opts, factor.WithCalculation(calculationAbility))
		}

		// 处理解读规则
		if len(factorPO.InterpretRules) > 0 {
			interpretRulePO := factorPO.InterpretRules[0]
			interpretRule := interpretation.NewInterpretRule(
				interpretation.NewScoreRange(
					interpretRulePO.ScoreRange.MinScore,
					interpretRulePO.ScoreRange.MaxScore,
				),
				interpretRulePO.Content,
			)
			interpretationAbility := &ability.InterpretationAbility{}
			interpretationAbility.SetInterpretationRule(&interpretRule)
			opts = append(opts, factor.WithInterpretation(interpretationAbility))
		}

		f := factor.NewFactor(
			factorPO.Code,
			factorPO.Title,
			factor.FactorType(factorPO.FactorType),
			opts...,
		)
		factors = append(factors, f)
	}

	return factors
}
