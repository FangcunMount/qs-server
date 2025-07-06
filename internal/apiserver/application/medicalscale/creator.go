package medicalscale

import (
	"context"
	"fmt"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/medicalscale"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/medicalscale/port"
	"github.com/yshujie/questionnaire-scale/pkg/log"
)

// Creator 医学量表创建器
type Creator struct {
	repo              port.Repository
	validationService *medicalscale.ValidationService
}

// NewCreator 创建医学量表创建器
func NewCreator(repo port.Repository, validationService *medicalscale.ValidationService) *Creator {
	return &Creator{
		repo:              repo,
		validationService: validationService,
	}
}

// CreateRequest 创建医学量表请求
type CreateRequest struct {
	Code                 string                `json:"code" binding:"required"`
	Title                string                `json:"title" binding:"required"`
	QuestionnaireCode    string                `json:"questionnaire_code" binding:"required"`
	QuestionnaireVersion string                `json:"questionnaire_version" binding:"required"`
	Factors              []CreateFactorRequest `json:"factors" binding:"required"`
}

// CreateFactorRequest 创建因子请求
type CreateFactorRequest struct {
	Code            string                       `json:"code" binding:"required"`
	Title           string                       `json:"title" binding:"required"`
	IsTotalScore    bool                         `json:"is_total_score"`
	Type            string                       `json:"type" binding:"required"`
	CalculationRule CreateCalculationRuleRequest `json:"calculation_rule" binding:"required"`
	InterpretRules  []CreateInterpretRuleRequest `json:"interpret_rules" binding:"required"`
}

// CreateCalculationRuleRequest 创建计算规则请求
type CreateCalculationRuleRequest struct {
	FormulaType string   `json:"formula_type" binding:"required"`
	SourceCodes []string `json:"source_codes" binding:"required"`
}

// CreateInterpretRuleRequest 创建解读规则请求
type CreateInterpretRuleRequest struct {
	MinScore float64 `json:"min_score" binding:"required"`
	MaxScore float64 `json:"max_score" binding:"required"`
	Content  string  `json:"content" binding:"required"`
}

// Create 创建医学量表
func (c *Creator) Create(ctx context.Context, req *CreateRequest) (*medicalscale.MedicalScale, error) {
	log.L(ctx).Infof("Creating medical scale: %s", req.Code)

	// 检查代码是否已存在
	exists, err := c.repo.ExistsByCode(ctx, req.Code)
	if err != nil {
		return nil, fmt.Errorf("failed to check if code exists: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("medical scale with code %s already exists", req.Code)
	}

	// 检查问卷绑定是否已存在
	exists, err = c.repo.ExistsByQuestionnaireBinding(ctx, req.QuestionnaireCode, req.QuestionnaireVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to check questionnaire binding: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("medical scale for questionnaire %s@%s already exists", req.QuestionnaireCode, req.QuestionnaireVersion)
	}

	// 验证问卷绑定
	if err := c.validationService.ValidateQuestionnaireBinding(ctx, req.QuestionnaireCode, req.QuestionnaireVersion); err != nil {
		return nil, fmt.Errorf("questionnaire binding validation failed: %w", err)
	}

	// 构建因子列表
	factors, err := c.buildFactors(ctx, req.Factors)
	if err != nil {
		return nil, fmt.Errorf("failed to build factors: %w", err)
	}

	// 创建医学量表
	scale := medicalscale.NewMedicalScale(
		medicalscale.NewMedicalScaleID(0), // ID将在保存时生成
		req.Code,
		req.Title,
		req.QuestionnaireCode,
		req.QuestionnaireVersion,
		factors,
	)

	// 验证量表
	if err := c.validationService.ValidateScale(ctx, scale); err != nil {
		return nil, fmt.Errorf("medical scale validation failed: %w", err)
	}

	// 保存到仓储
	if err := c.repo.Save(ctx, scale); err != nil {
		return nil, fmt.Errorf("failed to save medical scale: %w", err)
	}

	log.L(ctx).Infof("Medical scale created successfully: %s", req.Code)
	return scale, nil
}

// buildFactors 构建因子列表
func (c *Creator) buildFactors(ctx context.Context, factorReqs []CreateFactorRequest) ([]medicalscale.Factor, error) {
	var factors []medicalscale.Factor

	for i, factorReq := range factorReqs {
		factor, err := c.buildFactor(ctx, factorReq)
		if err != nil {
			return nil, fmt.Errorf("failed to build factor %d: %w", i, err)
		}
		factors = append(factors, factor)
	}

	return factors, nil
}

// buildFactor 构建单个因子
func (c *Creator) buildFactor(ctx context.Context, req CreateFactorRequest) (medicalscale.Factor, error) {
	// 解析因子类型
	factorType := medicalscale.FactorType(req.Type)
	if !factorType.IsValid() {
		return medicalscale.Factor{}, fmt.Errorf("invalid factor type: %s", req.Type)
	}

	// 构建计算规则
	calculationRule, err := c.buildCalculationRule(req.CalculationRule)
	if err != nil {
		return medicalscale.Factor{}, fmt.Errorf("failed to build calculation rule: %w", err)
	}

	// 构建解读规则列表
	interpretRules, err := c.buildInterpretRules(req.InterpretRules)
	if err != nil {
		return medicalscale.Factor{}, fmt.Errorf("failed to build interpret rules: %w", err)
	}

	// 创建因子
	factor := medicalscale.NewFactor(
		req.Code,
		req.Title,
		req.IsTotalScore,
		factorType,
		calculationRule,
		interpretRules,
	)

	// 验证因子
	if err := c.validationService.ValidateFactor(ctx, factor); err != nil {
		return medicalscale.Factor{}, fmt.Errorf("factor validation failed: %w", err)
	}

	return factor, nil
}

// buildCalculationRule 构建计算规则
func (c *Creator) buildCalculationRule(req CreateCalculationRuleRequest) (medicalscale.CalculationRule, error) {
	formulaType := medicalscale.FormulaType(req.FormulaType)
	if !formulaType.IsValid() {
		return medicalscale.CalculationRule{}, fmt.Errorf("invalid formula type: %s", req.FormulaType)
	}

	return medicalscale.NewCalculationRule(formulaType, req.SourceCodes), nil
}

// buildInterpretRules 构建解读规则列表
func (c *Creator) buildInterpretRules(reqs []CreateInterpretRuleRequest) ([]medicalscale.InterpretRule, error) {
	var rules []medicalscale.InterpretRule

	for i, req := range reqs {
		scoreRange := medicalscale.NewScoreRange(req.MinScore, req.MaxScore)
		if err := scoreRange.Validate(); err != nil {
			return nil, fmt.Errorf("invalid score range for rule %d: %w", i, err)
		}

		rule := medicalscale.NewInterpretRule(scoreRange, req.Content)
		rules = append(rules, rule)
	}

	// 检查规则是否有重叠
	for i, rule1 := range rules {
		for j, rule2 := range rules {
			if i != j && rule1.ScoreRange().IsOverlapping(rule2.ScoreRange()) {
				return nil, fmt.Errorf("interpret rules %d and %d have overlapping score ranges", i, j)
			}
		}
	}

	return rules, nil
}
