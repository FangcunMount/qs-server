package port

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/medicalscale"
)

// CalculationService 计算服务接口
type CalculationService interface {
	// CalculateFactorScore 计算因子分数
	CalculateFactorScore(ctx context.Context, factor medicalscale.Factor, answerValues map[string]interface{}) (float64, error)

	// CalculateScaleScores 计算量表所有因子分数
	CalculateScaleScores(ctx context.Context, scale *medicalscale.MedicalScale, answerValues map[string]interface{}) (map[string]float64, error)
}

// InterpretationService 解读服务接口
type InterpretationService interface {
	// GetFactorInterpretation 获取因子解读
	GetFactorInterpretation(ctx context.Context, factor medicalscale.Factor, score float64) (string, error)

	// GetScaleInterpretation 获取量表解读
	GetScaleInterpretation(ctx context.Context, scale *medicalscale.MedicalScale, scores map[string]float64) (map[string]string, error)
}

// ValidationService 验证服务接口
type ValidationService interface {
	// ValidateScale 验证量表完整性
	ValidateScale(ctx context.Context, scale *medicalscale.MedicalScale) error

	// ValidateFactor 验证因子完整性
	ValidateFactor(ctx context.Context, factor medicalscale.Factor) error

	// ValidateQuestionnaireBinding 验证问卷绑定
	ValidateQuestionnaireBinding(ctx context.Context, questionnaireCode, questionnaireVersion string) error
}
