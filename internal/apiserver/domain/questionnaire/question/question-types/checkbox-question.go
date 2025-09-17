package question_types

import (
    "github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire/question"
    "github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire/question/calculation"
    "github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire/question/option"
    "github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire/question/validation"
)

// CheckboxQuestion 多选问题
type CheckboxQuestion struct {
	BaseQuestion
	validation.ValidationAbility
	calculation.CalculationAbility

	options []option.Option
}

// 注册多选问题
func init() {
	RegisterQuestionFactory(question.QuestionTypeCheckbox, func(builder *QuestionBuilder) question.Question {
		// 创建多选问题
		q := newCheckboxQuestion(builder.GetCode(), builder.GetTitle())

		// 设置选项
		q.setOptions(builder.GetOptions())

		// 设置校验规则
		for _, rule := range builder.GetValidationRules() {
			q.addValidationRule(rule)
		}

		// 设置计算规则
		if builder.GetCalculationRule() != nil {
			q.setCalculationRule(builder.GetCalculationRule())
		}

		return q
	})
}

// NewCheckboxQuestion 创建多选问题
func newCheckboxQuestion(code question.QuestionCode, title string) *CheckboxQuestion {
	return &CheckboxQuestion{
		BaseQuestion: NewBaseQuestion(code, title, question.QuestionTypeCheckbox),
	}
}

// setOptions 设置选项
func (q *CheckboxQuestion) setOptions(options []option.Option) {
	q.options = options
}

// addValidationRule 添加校验规则
func (q *CheckboxQuestion) addValidationRule(rule validation.ValidationRule) {
	q.ValidationAbility.AddValidationRule(rule)
}

// setCalculationRule 设置计算规则
func (q *CheckboxQuestion) setCalculationRule(rule *calculation.CalculationRule) {
	q.CalculationAbility.SetCalculationRule(rule)
}

// GetOptions 获取选项
func (q *CheckboxQuestion) GetOptions() []option.Option {
	return q.options
}

// GetValidationRules 获取校验规则 - 重写BaseQuestion的默认实现
func (q *CheckboxQuestion) GetValidationRules() []validation.ValidationRule {
	return q.ValidationAbility.GetValidationRules()
}

// GetCalculationRule 获取计算规则 - 重写BaseQuestion的默认实现
func (q *CheckboxQuestion) GetCalculationRule() *calculation.CalculationRule {
	return q.CalculationAbility.GetCalculationRule()
}
