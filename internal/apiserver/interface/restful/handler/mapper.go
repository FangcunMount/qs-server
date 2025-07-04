package handler

import (
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question/calculation"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question/option"
	question_types "github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question/question-types"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question/validation"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/interface/restful/dto"
	"github.com/yshujie/questionnaire-scale/pkg/log"
)

// QuestionMapper 问题映射器
type QuestionMapper struct{}

// NewQuestionMapper 创建问题映射器
func NewQuestionMapper() *QuestionMapper {
	return &QuestionMapper{}
}

// mapQuestionsToBOs 将问题 DTO 列表转为领域对象列表
func (m *QuestionMapper) mapQuestionsToBOs(questions []dto.Question) []question.Question {
	boQuestions := make([]question.Question, 0, len(questions))
	for _, q := range questions {
		if bo := m.mapQuestionToBO(q); bo != nil {
			boQuestions = append(boQuestions, bo)
		}
	}
	return boQuestions
}

// mapQuestionToBO 将问题 DTO 转为领域对象
func (m *QuestionMapper) mapQuestionToBO(q dto.Question) question.Question {
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
	result := question_types.NewQuestionFactory().CreateFromBuilder(builder)
	if result == nil {
		log.Errorw("---- CreateFromBuilder returned nil for question:", "code", q.Code, "type", q.Type)
	} else {
		log.Infow("---- Successfully created question:", "code", result.GetCode().Value(), "type", result.GetType())
	}

	return result
}

// mapOptionsToBO 将选项DTO转换为领域模型
func (m *QuestionMapper) mapOptionsToBO(options []dto.Option) []option.Option {
	opts := make([]option.Option, len(options))
	for i, o := range options {
		opts[i] = option.NewOption(o.Code, o.Content, o.Score)
	}
	return opts
}

// mapValidationRulesToBO 将校验规则DTO转换为领域模型
func (m *QuestionMapper) mapValidationRulesToBO(rules []dto.ValidationRule) []validation.ValidationRule {
	opts := make([]validation.ValidationRule, len(rules))
	for i, r := range rules {
		opts[i] = validation.NewValidationRule(validation.RuleType(r.RuleType), r.TargetValue)
	}
	return opts
}

// mapQuestionsToDTOs 将 Questions 转为 question DTO 列表
func (m *QuestionMapper) mapQuestionsToDTOs(questions []question.Question) []dto.Question {
	dtos := make([]dto.Question, len(questions))
	for i, q := range questions {
		dtos[i] = m.mapQuestionToDTO(q)
	}
	return dtos
}

// mapQuestionToDTO 将 question 转为 question DTO
func (m *QuestionMapper) mapQuestionToDTO(q question.Question) dto.Question {
	qDto := dto.Question{
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
func (m *QuestionMapper) mapOptionsToDTO(options []option.Option) []dto.Option {
	dtos := make([]dto.Option, len(options))
	for i, o := range options {
		dtos[i] = dto.Option{
			Code:    o.GetCode(),
			Content: o.GetContent(),
			Score:   o.GetScore(),
		}
	}
	return dtos
}

// mapValidationRulesToDTO 将 validationRules 转为 validationRule DTO 列表
func (m *QuestionMapper) mapValidationRulesToDTO(rules []validation.ValidationRule) []dto.ValidationRule {
	dtos := make([]dto.ValidationRule, len(rules))
	for i, r := range rules {
		dtos[i] = dto.ValidationRule{
			RuleType:    string(r.GetRuleType()),
			TargetValue: r.GetTargetValue(),
		}
	}
	return dtos
}

// mapCalculationRuleToDTO 将 calculationRule 转为 calculationRule DTO
func (m *QuestionMapper) mapCalculationRuleToDTO(rule *calculation.CalculationRule) *dto.CalculationRule {
	if rule == nil {
		return nil
	}
	return &dto.CalculationRule{
		FormulaType: string(rule.GetFormulaType()),
	}
}
