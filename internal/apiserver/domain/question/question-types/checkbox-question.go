package question_types

import (
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question/calculation"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question/option"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question/validation"
)

// CheckboxQuestion 多选问题
type CheckboxQuestion struct {
	BaseQuestion
	validation.ValidationAbility
	calculation.CalculationAbility

	options []option.Option
}

// NewCheckboxQuestion 创建多选问题
func NewCheckboxQuestion(code question.QuestionCode, title string) *CheckboxQuestion {
	return &CheckboxQuestion{
		BaseQuestion: NewBaseQuestion(code, title, question.QuestionTypeCheckbox),
	}
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

// SetOptions 设置选项
func (q *CheckboxQuestion) SetOptions(options []option.Option) {
	q.options = options
}

// AddOption 添加选项
func (q *CheckboxQuestion) AddOption(option option.Option) {
	// 如果选项已存在，则不添加
	for _, o := range q.options {
		if o.GetCode() == option.GetCode() {
			return
		}
	}

	// 如果选项不存在，则添加
	q.options = append(q.options, option)
}

// ClearOptions 清空选项
func (q *CheckboxQuestion) ClearOptions() {
	q.options = []option.Option{}
}

// AddValidationRule 添加校验规则
func (q *CheckboxQuestion) AddValidationRule(rule validation.ValidationRule) {
	q.ValidationAbility.AddValidationRule(rule)
}

// SetCalculationRule 设置计算规则
func (q *CheckboxQuestion) SetCalculationRule(rule *calculation.CalculationRule) {
	q.CalculationAbility.SetCalculationRule(rule)
}
