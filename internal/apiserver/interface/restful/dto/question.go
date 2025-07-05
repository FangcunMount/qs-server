package dto

import (
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question/calculation"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question/option"
	question_types "github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question/question-types"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question/validation"
	"github.com/yshujie/questionnaire-scale/pkg/log"
)

// Question 问题
type Question struct {
	Code  string `json:"code"`          // 问题ID，仅更新/编辑时提供
	Type  string `json:"question_type"` // 问题题型：single_choice, multi_choice, text 等
	Title string `json:"title"`         // 问题主标题
	Tips  string `json:"tips"`          // 问题提示

	// 特定属性
	Placeholder string   `json:"placeholder"`       // 问题占位符
	Options     []Option `json:"options,omitempty"` // 问题选项（可选项，结构化题型）

	// 能力属性
	ValidationRules []ValidationRule `json:"validation_rules,omitempty"` // 校验规则（可选项）
	CalculationRule *CalculationRule `json:"calculation_rule,omitempty"` // 问题算分规则（可选项，结构化题型）
}

// Option 选项
type Option struct {
	Code    string `json:"code"`    // 选项ID，仅更新/编辑时提供
	Content string `json:"content"` // 选项内容
	Score   int    `json:"score"`   // 选项分数
}

// ValidationRule 校验规则
type ValidationRule struct {
	RuleType    string `json:"rule_type"`    // 规则类型
	TargetValue string `json:"target_value"` // 目标值
}

// CalculationRule 算分规则
type CalculationRule struct {
	FormulaType string `json:"formula_type"` // 公式类型
}

// QuestionMapper 问题映射器
type QuestionMapper struct{}

// NewQuestionMapper 创建问题映射器
func NewQuestionMapper() *QuestionMapper {
	return &QuestionMapper{}
}

// mapQuestionsToBOs 将问题 DTO 列表转为领域对象列表
func (m *QuestionMapper) MapQuestionsToBOs(questions []Question) []question.Question {
	boQuestions := make([]question.Question, 0, len(questions))
	for _, q := range questions {
		if bo := m.mapQuestionToBO(q); bo != nil {
			boQuestions = append(boQuestions, bo)
		}
	}
	return boQuestions
}

// mapQuestionToBO 将问题 DTO 转为领域对象
func (m *QuestionMapper) mapQuestionToBO(q Question) question.Question {
	log.Infow("---- mapQuestionToBO input:", "code", q.Code, "type", q.Type, "title", q.Title)

	// 构建配置选项列表
	opts := []question_types.BuilderOption{
		question_types.WithCode(question.NewQuestionCode(q.Code)),
		question_types.WithTitle(q.Title),
		question_types.WithTips(q.Tips),
		question_types.WithQuestionType(question.QuestionType(q.Type)),
	}

	// 添加特定属性
	if q.Placeholder != "" {
		opts = append(opts, question_types.WithPlaceholder(q.Placeholder))
	}
	if len(q.Options) > 0 {
		opts = append(opts, question_types.WithOptions(m.mapOptionsToBO(q.Options)))
	}
	if len(q.ValidationRules) > 0 {
		opts = append(opts, question_types.WithValidationRules(m.mapValidationRulesToBO(q.ValidationRules)))
	}
	if q.CalculationRule != nil {
		opts = append(opts, question_types.WithCalculationRule(calculation.FormulaType(q.CalculationRule.FormulaType)))
	}

	// 1. 创建配置
	builder := question_types.BuildQuestionConfig(opts...)

	// 验证 builder 有效性
	if !builder.IsValid() {
		log.Errorw("---- QuestionBuilder validation failed:", "errors", builder.GetValidationErrors())
		return nil
	}

	// 2. 创建对象
	result := question_types.CreateQuestionFromBuilder(builder)
	if result == nil {
		log.Errorw("---- CreateFromBuilder returned nil for question:", "code", q.Code, "type", q.Type)
	} else {
		log.Infow("---- Successfully created question:", "code", result.GetCode().Value(), "type", result.GetType())
	}

	return result
}

// mapOptionsToBO 将选项DTO转换为领域模型
func (m *QuestionMapper) mapOptionsToBO(options []Option) []option.Option {
	opts := make([]option.Option, len(options))
	for i, o := range options {
		opts[i] = option.NewOption(o.Code, o.Content, o.Score)
	}
	return opts
}

// mapValidationRulesToBO 将校验规则DTO转换为领域模型
func (m *QuestionMapper) mapValidationRulesToBO(rules []ValidationRule) []validation.ValidationRule {
	opts := make([]validation.ValidationRule, len(rules))
	for i, r := range rules {
		opts[i] = validation.NewValidationRule(validation.RuleType(r.RuleType), r.TargetValue)
	}
	return opts
}

// MapQuestionsToDTOs 将 Questions 转为 question DTO 列表
func (m *QuestionMapper) MapQuestionsToDTOs(questions []question.Question) []Question {
	dtos := make([]Question, len(questions))
	for i, q := range questions {
		dtos[i] = m.mapQuestionToDTO(q)
	}
	return dtos
}

// mapQuestionToDTO 将 question 转为 question DTO
func (m *QuestionMapper) mapQuestionToDTO(q question.Question) Question {
	qDto := Question{
		Code:  q.GetCode().Value(),
		Type:  string(q.GetType()),
		Title: q.GetTitle(),
		Tips:  q.GetTips(),
	}

	// 添加特定属性
	if q.GetPlaceholder() != "" {
		qDto.Placeholder = q.GetPlaceholder()
	}
	if q.GetOptions() != nil {
		qDto.Options = m.mapOptionsToDTO(q.GetOptions())
	}
	if q.GetValidationRules() != nil {
		qDto.ValidationRules = m.mapValidationRulesToDTO(q.GetValidationRules())
	}
	if q.GetCalculationRule() != nil {
		qDto.CalculationRule = m.mapCalculationRuleToDTO(q.GetCalculationRule())
	}
	return qDto
}

// mapOptionsToDTO 将 options 转为 option DTO 列表
func (m *QuestionMapper) mapOptionsToDTO(options []option.Option) []Option {
	dtos := make([]Option, len(options))
	for i, o := range options {
		dtos[i] = Option{
			Code:    o.GetCode(),
			Content: o.GetContent(),
			Score:   o.GetScore(),
		}
	}
	return dtos
}

// mapValidationRulesToDTO 将 validationRules 转为 validationRule DTO 列表
func (m *QuestionMapper) mapValidationRulesToDTO(rules []validation.ValidationRule) []ValidationRule {
	dtos := make([]ValidationRule, len(rules))
	for i, r := range rules {
		dtos[i] = ValidationRule{
			RuleType:    string(r.GetRuleType()),
			TargetValue: r.GetTargetValue(),
		}
	}
	return dtos
}

// mapCalculationRuleToDTO 将 calculationRule 转为 calculationRule DTO
func (m *QuestionMapper) mapCalculationRuleToDTO(rule *calculation.CalculationRule) *CalculationRule {
	if rule == nil {
		return nil
	}
	return &CalculationRule{
		FormulaType: string(rule.GetFormulaType()),
	}
}
