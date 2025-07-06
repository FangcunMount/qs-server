package medicalscale

import (
	"context"
	"fmt"
	"strconv"

	"github.com/yshujie/questionnaire-scale/pkg/log"
)

// CalculationService 计算服务实现
type CalculationService struct{}

// NewCalculationService 创建计算服务
func NewCalculationService() *CalculationService {
	return &CalculationService{}
}

// CalculateFactorScore 计算因子分数
func (cs *CalculationService) CalculateFactorScore(ctx context.Context, factor Factor, answerValues map[string]interface{}) (float64, error) {
	log.L(ctx).Infof("Calculating score for factor: %s", factor.Code())

	rule := factor.CalculationRule()
	sourceCodes := rule.SourceCodes()

	// 收集源数据
	var values []float64
	for _, sourceCode := range sourceCodes {
		value, exists := answerValues[sourceCode]
		if !exists {
			log.L(ctx).Warnf("Source code %s not found in answer values", sourceCode)
			continue
		}

		floatValue, err := cs.convertToFloat(value)
		if err != nil {
			return 0, fmt.Errorf("failed to convert value for source %s: %w", sourceCode, err)
		}

		values = append(values, floatValue)
	}

	if len(values) == 0 {
		return 0, fmt.Errorf("no valid values found for factor %s", factor.Code())
	}

	// 根据公式类型计算分数
	var score float64
	var err error

	switch rule.FormulaType() {
	case SumFormula:
		score = cs.calculateSum(values)
	case AverageFormula:
		score = cs.calculateAverage(values)
	case WeightedSumFormula:
		// 加权求和需要额外的权重信息，这里简化为普通求和
		score = cs.calculateSum(values)
		log.L(ctx).Warn("Weighted sum formula not fully implemented, using sum instead")
	case CustomFormula:
		// 自定义公式需要额外的公式定义，这里简化为求和
		score = cs.calculateSum(values)
		log.L(ctx).Warn("Custom formula not implemented, using sum instead")
	default:
		return 0, fmt.Errorf("unsupported formula type: %s", rule.FormulaType())
	}

	log.L(ctx).Infof("Factor %s score calculated: %.2f", factor.Code(), score)
	return score, err
}

// CalculateScaleScores 计算量表所有因子分数
func (cs *CalculationService) CalculateScaleScores(ctx context.Context, scale *MedicalScale, answerValues map[string]interface{}) (map[string]float64, error) {
	log.L(ctx).Infof("Calculating scores for medical scale: %s", scale.Code())

	scores := make(map[string]float64)
	factors := scale.Factors()

	for _, factor := range factors {
		score, err := cs.CalculateFactorScore(ctx, factor, answerValues)
		if err != nil {
			log.L(ctx).Errorf("Failed to calculate score for factor %s: %v", factor.Code(), err)
			// 继续计算其他因子，不因为一个因子失败而停止
			scores[factor.Code()] = 0
			continue
		}
		scores[factor.Code()] = score
	}

	log.L(ctx).Infof("Medical scale %s scores calculated: %v", scale.Code(), scores)
	return scores, nil
}

// convertToFloat 将值转换为浮点数
func (cs *CalculationService) convertToFloat(value interface{}) (float64, error) {
	switch v := value.(type) {
	case float64:
		return v, nil
	case float32:
		return float64(v), nil
	case int:
		return float64(v), nil
	case int32:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case string:
		return strconv.ParseFloat(v, 64)
	default:
		return 0, fmt.Errorf("cannot convert %T to float64", value)
	}
}

// calculateSum 计算求和
func (cs *CalculationService) calculateSum(values []float64) float64 {
	var sum float64
	for _, value := range values {
		sum += value
	}
	return sum
}

// calculateAverage 计算平均值
func (cs *CalculationService) calculateAverage(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	return cs.calculateSum(values) / float64(len(values))
}

// InterpretationService 解读服务实现
type InterpretationService struct{}

// NewInterpretationService 创建解读服务
func NewInterpretationService() *InterpretationService {
	return &InterpretationService{}
}

// GetFactorInterpretation 获取因子解读
func (is *InterpretationService) GetFactorInterpretation(ctx context.Context, factor Factor, score float64) (string, error) {
	log.L(ctx).Infof("Getting interpretation for factor %s with score %.2f", factor.Code(), score)

	interpretation, err := factor.GetInterpretation(score)
	if err != nil {
		log.L(ctx).Errorf("Failed to get interpretation for factor %s: %v", factor.Code(), err)
		return "", err
	}

	log.L(ctx).Infof("Factor %s interpretation: %s", factor.Code(), interpretation)
	return interpretation, nil
}

// GetScaleInterpretation 获取量表解读
func (is *InterpretationService) GetScaleInterpretation(ctx context.Context, scale *MedicalScale, scores map[string]float64) (map[string]string, error) {
	log.L(ctx).Infof("Getting interpretations for medical scale: %s", scale.Code())

	interpretations := make(map[string]string)
	factors := scale.Factors()

	for _, factor := range factors {
		score, exists := scores[factor.Code()]
		if !exists {
			log.L(ctx).Warnf("Score not found for factor %s", factor.Code())
			interpretations[factor.Code()] = "分数缺失，无法解读"
			continue
		}

		interpretation, err := is.GetFactorInterpretation(ctx, factor, score)
		if err != nil {
			log.L(ctx).Errorf("Failed to get interpretation for factor %s: %v", factor.Code(), err)
			interpretations[factor.Code()] = "解读失败"
			continue
		}

		interpretations[factor.Code()] = interpretation
	}

	log.L(ctx).Infof("Medical scale %s interpretations completed", scale.Code())
	return interpretations, nil
}

// ValidationService 验证服务实现
type ValidationService struct{}

// NewValidationService 创建验证服务
func NewValidationService() *ValidationService {
	return &ValidationService{}
}

// ValidateScale 验证量表完整性
func (vs *ValidationService) ValidateScale(ctx context.Context, scale *MedicalScale) error {
	log.L(ctx).Infof("Validating medical scale: %s", scale.Code())

	if err := scale.Validate(); err != nil {
		log.L(ctx).Errorf("Medical scale validation failed: %v", err)
		return err
	}

	log.L(ctx).Infof("Medical scale %s validation passed", scale.Code())
	return nil
}

// ValidateFactor 验证因子完整性
func (vs *ValidationService) ValidateFactor(ctx context.Context, factor Factor) error {
	log.L(ctx).Infof("Validating factor: %s", factor.Code())

	if err := factor.Validate(); err != nil {
		log.L(ctx).Errorf("Factor validation failed: %v", err)
		return err
	}

	log.L(ctx).Infof("Factor %s validation passed", factor.Code())
	return nil
}

// ValidateQuestionnaireBinding 验证问卷绑定
func (vs *ValidationService) ValidateQuestionnaireBinding(ctx context.Context, questionnaireCode, questionnaireVersion string) error {
	log.L(ctx).Infof("Validating questionnaire binding: %s@%s", questionnaireCode, questionnaireVersion)

	if questionnaireCode == "" {
		return fmt.Errorf("questionnaire code cannot be empty")
	}
	if questionnaireVersion == "" {
		return fmt.Errorf("questionnaire version cannot be empty")
	}

	// 这里可以添加更多的验证逻辑，比如检查问卷是否存在
	// 由于需要调用问卷领域的服务，这里暂时简化

	log.L(ctx).Infof("Questionnaire binding validation passed: %s@%s", questionnaireCode, questionnaireVersion)
	return nil
}
