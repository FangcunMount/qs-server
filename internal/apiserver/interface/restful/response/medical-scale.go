package response

import (
	medicalScale "github.com/yshujie/questionnaire-scale/internal/apiserver/domain/medical-scale"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/medical-scale/factor"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/interface/restful/viewmodel"
)

// MedicalScaleResponse 医学量表响应
type MedicalScaleResponse struct {
	Data *viewmodel.MedicalScaleVM `json:"data"`
}

// NewMedicalScaleResponse 创建医学量表响应
func NewMedicalScaleResponse(scale *medicalScale.MedicalScale) *MedicalScaleResponse {
	if scale == nil {
		return &MedicalScaleResponse{
			Data: nil,
		}
	}

	return &MedicalScaleResponse{
		Data: &viewmodel.MedicalScaleVM{
			ID:                scale.GetID().Value(),
			Code:              scale.GetCode(),
			Title:             scale.GetTitle(),
			QuestionnaireCode: scale.GetQuestionnaireCode(),
			Factors:           mapFactorsToVM(scale.GetFactors()),
		},
	}
}

// mapFactorsToVM 将因子领域对象转换为视图模型
func mapFactorsToVM(factors []factor.Factor) []viewmodel.FactorVM {
	if factors == nil {
		return []viewmodel.FactorVM{}
	}

	var factorsVM []viewmodel.FactorVM
	for _, f := range factors {
		factorVM := viewmodel.FactorVM{
			Code:       f.GetCode(),
			Title:      f.GetTitle(),
			FactorType: f.GetFactorType().String(),
		}

		// 处理计算能力
		if calcAbility := f.GetCalculationAbility(); calcAbility != nil {
			if calcRule := calcAbility.GetCalculationRule(); calcRule != nil {
				factorVM.CalculationRule = viewmodel.CalculationRuleVM{
					FormulaType: calcRule.GetFormula().String(),
					SourceCodes: calcRule.GetSourceCodes(),
				}
			}
		}

		// 处理解读能力
		if interpretAbility := f.GetInterpretationAbility(); interpretAbility != nil {
			if interpretRule := interpretAbility.GetInterpretationRule(); interpretRule != nil {
				factorVM.InterpretRules = []viewmodel.InterpretRuleVM{
					{
						ScoreRange: viewmodel.ScoreRangeVM{
							MinScore: interpretRule.GetScoreRange().MinScore(),
							MaxScore: interpretRule.GetScoreRange().MaxScore(),
						},
						Content: interpretRule.GetContent(),
					},
				}
			}
		}

		factorsVM = append(factorsVM, factorVM)
	}

	return factorsVM
}
