package mapper

import (
	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/dto"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/medicalscale"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/medicalscale/factor"
	"github.com/yshujie/questionnaire-scale/internal/pkg/calculation"
	"github.com/yshujie/questionnaire-scale/internal/pkg/interpretation"
)

// MedicalScaleMapper 医学量表映射器
type MedicalScaleMapper struct{}

// NewMedicalScaleMapper 创建医学量表映射器
func NewMedicalScaleMapper() MedicalScaleMapper {
	return MedicalScaleMapper{}
}

func (m *MedicalScaleMapper) ToDTO(bo *medicalscale.MedicalScale) *dto.MedicalScaleDTO {
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
		dtos[i] = dto.FactorDTO{
			Code:            factor.GetCode(),
			Title:           factor.GetTitle(),
			FactorType:      string(factor.GetFactorType()),
			CalculationRule: m.toCalculationRuleDTO(factor.GetCalculationAbility().GetCalculationRule()),
			InterpretRule:   m.toInterpretRuleDTO(factor.GetInterpretationAbility().GetInterpretationRule()),
		}
	}
	return dtos
}

// toCalculationRuleDTO 将计算规则领域对象转换为 DTO
func (m *MedicalScaleMapper) toCalculationRuleDTO(rule *calculation.CalculationRule) *dto.CalculationRuleDTO {
	return &dto.CalculationRuleDTO{
		FormulaType: rule.GetFormula().String(),
		SourceCodes: rule.GetSourceCodes(),
	}
}

// toInterpretRuleDTO 将解读规则领域对象转换为 DTO
func (m *MedicalScaleMapper) toInterpretRuleDTO(rule *interpretation.InterpretRule) *dto.InterpretRuleDTO {
	return &dto.InterpretRuleDTO{
		ScoreRange: dto.ScoreRangeDTO{
			MinScore: rule.GetScoreRange().MinScore(),
			MaxScore: rule.GetScoreRange().MaxScore(),
		},
		Content: rule.GetContent(),
	}
}
