package mapper

import (
	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/dto"
	medicalScale "github.com/yshujie/questionnaire-scale/internal/apiserver/domain/medical-scale"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/medical-scale/factor"
	"github.com/yshujie/questionnaire-scale/internal/pkg/calculation"
	"github.com/yshujie/questionnaire-scale/internal/pkg/interpretation"
)

// MedicalScaleMapper 医学量表映射器
type MedicalScaleMapper struct{}

// NewMedicalScaleMapper 创建医学量表映射器
func NewMedicalScaleMapper() MedicalScaleMapper {
	return MedicalScaleMapper{}
}

func (m *MedicalScaleMapper) ToDTO(bo *medicalScale.MedicalScale) *dto.MedicalScaleDTO {
	return &dto.MedicalScaleDTO{
		ID:                bo.GetID().Value(),
		Code:              bo.GetCode(),
		QuestionnaireCode: bo.GetQuestionnaireCode(),
		Title:             bo.GetTitle(),
		Description:       bo.GetDescription(),
		Factors:           m.toFactorDTOs(bo.GetFactors()),
	}
}

// toFactorDTOs 将因子领域对象转换为 DTO
func (m *MedicalScaleMapper) toFactorDTOs(factors []factor.Factor) []dto.FactorDTO {
	dtos := make([]dto.FactorDTO, len(factors))
	for i, factor := range factors {
		var calculationRule *dto.CalculationRuleDTO
		if factor.GetCalculationAbility() != nil {
			calculationRule = m.toCalculationRuleDTO(factor.GetCalculationAbility().GetCalculationRule())
		}

		var interpretRules []dto.InterpretRuleDTO
		if factor.GetInterpretationAbility() != nil {
			interpretRules = m.toInterpretRuleDTOs(factor.GetInterpretationAbility().GetInterpretationRules())
		}

		dtos[i] = dto.FactorDTO{
			Code:            factor.GetCode(),
			Title:           factor.GetTitle(),
			FactorType:      string(factor.GetFactorType()),
			IsTotalScore:    factor.IsTotalScore(),
			CalculationRule: calculationRule,
			InterpretRules:  interpretRules,
		}
	}
	return dtos
}

// toCalculationRuleDTO 将计算规则领域对象转换为 DTO
func (m *MedicalScaleMapper) toCalculationRuleDTO(rule *calculation.CalculationRule) *dto.CalculationRuleDTO {
	if rule == nil {
		return nil
	}
	return &dto.CalculationRuleDTO{
		FormulaType: rule.GetFormula().String(),
		SourceCodes: rule.GetSourceCodes(),
	}
}

// toInterpretRuleDTOs 将解读规则领域对象转换为 DTO 数组
func (m *MedicalScaleMapper) toInterpretRuleDTOs(rules []interpretation.InterpretRule) []dto.InterpretRuleDTO {
	if len(rules) == 0 {
		return nil
	}

	dtos := make([]dto.InterpretRuleDTO, len(rules))
	for i, rule := range rules {
		dtos[i] = dto.InterpretRuleDTO{
			ScoreRange: dto.ScoreRangeDTO{
				MinScore: rule.GetScoreRange().MinScore(),
				MaxScore: rule.GetScoreRange().MaxScore(),
			},
			Content: rule.GetContent(),
		}
	}
	return dtos
}
