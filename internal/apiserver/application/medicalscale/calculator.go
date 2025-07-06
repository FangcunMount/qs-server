package medicalscale

import (
	"context"
	"fmt"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/medicalscale"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/medicalscale/port"
	"github.com/yshujie/questionnaire-scale/pkg/log"
)

// Calculator 医学量表计算器
type Calculator struct {
	repo                  port.Repository
	calculationService    *medicalscale.CalculationService
	interpretationService *medicalscale.InterpretationService
}

// NewCalculator 创建医学量表计算器
func NewCalculator(
	repo port.Repository,
	calculationService *medicalscale.CalculationService,
	interpretationService *medicalscale.InterpretationService,
) *Calculator {
	return &Calculator{
		repo:                  repo,
		calculationService:    calculationService,
		interpretationService: interpretationService,
	}
}

// CalculateRequest 计算请求
type CalculateRequest struct {
	MedicalScaleCode string                 `json:"medical_scale_code" binding:"required"`
	AnswerValues     map[string]interface{} `json:"answer_values" binding:"required"`
}

// CalculateResponse 计算响应
type CalculateResponse struct {
	MedicalScaleCode  string             `json:"medical_scale_code"`
	MedicalScaleTitle string             `json:"medical_scale_title"`
	FactorScores      map[string]float64 `json:"factor_scores"`
	Interpretations   map[string]string  `json:"interpretations"`
	FactorDetails     []FactorDetail     `json:"factor_details"`
}

// FactorDetail 因子详情
type FactorDetail struct {
	Code           string  `json:"code"`
	Title          string  `json:"title"`
	Type           string  `json:"type"`
	IsTotalScore   bool    `json:"is_total_score"`
	Score          float64 `json:"score"`
	Interpretation string  `json:"interpretation"`
}

// Calculate 计算医学量表分数和解读
func (c *Calculator) Calculate(ctx context.Context, req *CalculateRequest) (*CalculateResponse, error) {
	log.L(ctx).Infof("Calculating medical scale: %s", req.MedicalScaleCode)

	// 获取医学量表
	scale, err := c.repo.FindByCode(ctx, req.MedicalScaleCode)
	if err != nil {
		return nil, fmt.Errorf("failed to find medical scale: %w", err)
	}

	// 计算所有因子分数
	scores, err := c.calculationService.CalculateScaleScores(ctx, scale, req.AnswerValues)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate scale scores: %w", err)
	}

	// 获取所有因子解读
	interpretations, err := c.interpretationService.GetScaleInterpretation(ctx, scale, scores)
	if err != nil {
		return nil, fmt.Errorf("failed to get scale interpretations: %w", err)
	}

	// 构建因子详情
	factorDetails := c.buildFactorDetails(scale.Factors(), scores, interpretations)

	response := &CalculateResponse{
		MedicalScaleCode:  scale.Code(),
		MedicalScaleTitle: scale.Title(),
		FactorScores:      scores,
		Interpretations:   interpretations,
		FactorDetails:     factorDetails,
	}

	log.L(ctx).Infof("Medical scale calculation completed: %s", req.MedicalScaleCode)
	return response, nil
}

// CalculateFactor 计算单个因子分数
func (c *Calculator) CalculateFactor(ctx context.Context, medicalScaleCode, factorCode string, answerValues map[string]interface{}) (float64, string, error) {
	log.L(ctx).Infof("Calculating factor %s in medical scale %s", factorCode, medicalScaleCode)

	// 获取医学量表
	scale, err := c.repo.FindByCode(ctx, medicalScaleCode)
	if err != nil {
		return 0, "", fmt.Errorf("failed to find medical scale: %w", err)
	}

	// 获取因子
	factor, err := scale.GetFactor(factorCode)
	if err != nil {
		return 0, "", fmt.Errorf("failed to get factor: %w", err)
	}

	// 计算因子分数
	score, err := c.calculationService.CalculateFactorScore(ctx, factor, answerValues)
	if err != nil {
		return 0, "", fmt.Errorf("failed to calculate factor score: %w", err)
	}

	// 获取因子解读
	interpretation, err := c.interpretationService.GetFactorInterpretation(ctx, factor, score)
	if err != nil {
		return score, "", fmt.Errorf("failed to get factor interpretation: %w", err)
	}

	log.L(ctx).Infof("Factor calculation completed: %s = %.2f", factorCode, score)
	return score, interpretation, nil
}

// GetFactorInterpretation 获取因子解读
func (c *Calculator) GetFactorInterpretation(ctx context.Context, medicalScaleCode, factorCode string, score float64) (string, error) {
	log.L(ctx).Infof("Getting interpretation for factor %s with score %.2f", factorCode, score)

	// 获取医学量表
	scale, err := c.repo.FindByCode(ctx, medicalScaleCode)
	if err != nil {
		return "", fmt.Errorf("failed to find medical scale: %w", err)
	}

	// 获取因子
	factor, err := scale.GetFactor(factorCode)
	if err != nil {
		return "", fmt.Errorf("failed to get factor: %w", err)
	}

	// 获取解读
	interpretation, err := c.interpretationService.GetFactorInterpretation(ctx, factor, score)
	if err != nil {
		return "", fmt.Errorf("failed to get factor interpretation: %w", err)
	}

	log.L(ctx).Infof("Factor interpretation: %s", interpretation)
	return interpretation, nil
}

// ValidateAnswerValues 验证答案值
func (c *Calculator) ValidateAnswerValues(ctx context.Context, medicalScaleCode string, answerValues map[string]interface{}) error {
	log.L(ctx).Infof("Validating answer values for medical scale: %s", medicalScaleCode)

	// 获取医学量表
	scale, err := c.repo.FindByCode(ctx, medicalScaleCode)
	if err != nil {
		return fmt.Errorf("failed to find medical scale: %w", err)
	}

	// 收集所有需要的源代码
	requiredCodes := make(map[string]bool)
	for _, factor := range scale.Factors() {
		for _, sourceCode := range factor.CalculationRule().SourceCodes() {
			requiredCodes[sourceCode] = true
		}
	}

	// 检查缺失的答案值
	var missingCodes []string
	for code := range requiredCodes {
		if _, exists := answerValues[code]; !exists {
			missingCodes = append(missingCodes, code)
		}
	}

	if len(missingCodes) > 0 {
		return fmt.Errorf("missing answer values for codes: %v", missingCodes)
	}

	log.L(ctx).Infof("Answer values validation passed for medical scale: %s", medicalScaleCode)
	return nil
}

// buildFactorDetails 构建因子详情列表
func (c *Calculator) buildFactorDetails(factors []medicalscale.Factor, scores map[string]float64, interpretations map[string]string) []FactorDetail {
	var details []FactorDetail

	for _, factor := range factors {
		score := scores[factor.Code()]
		interpretation := interpretations[factor.Code()]

		detail := FactorDetail{
			Code:           factor.Code(),
			Title:          factor.Title(),
			Type:           factor.Type().String(),
			IsTotalScore:   factor.IsTotalScore(),
			Score:          score,
			Interpretation: interpretation,
		}

		details = append(details, detail)
	}

	return details
}
