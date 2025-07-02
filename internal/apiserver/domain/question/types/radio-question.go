package types

import (
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question/calculation"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question/option"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question/validation"
)

// RadioQuestion 单选问题
type RadioQuestion struct {
	BaseQuestion
	validation.ValidationAbility
	calculation.CalculationAbility

	options []option.Option
}

// NewRadioQuestion 创建单选问题
func NewRadioQuestion(code question.QuestionCode, title string) *RadioQuestion {
	return &RadioQuestion{
		BaseQuestion: NewBaseQuestion(code, title, question.QuestionTypeRadio),
	}
}

// GetOptions 获取选项
func (q *RadioQuestion) GetOptions() []option.Option {
	return q.options
}

// GetValidationRules 获取校验规则 - 重写BaseQuestion的默认实现
func (q *RadioQuestion) GetValidationRules() []validation.ValidationRule {
	return q.ValidationAbility.GetValidationRules()
}

// GetCalculationRule 获取计算规则 - 重写BaseQuestion的默认实现
func (q *RadioQuestion) GetCalculationRule() *calculation.CalculationRule {
	return q.CalculationAbility.GetCalculationRule()
}

// SetOptions 设置选项
func (q *RadioQuestion) SetOptions(options []option.Option) {
	q.options = options
}

// AddOption 添加选项
func (q *RadioQuestion) AddOption(option option.Option) {
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
func (q *RadioQuestion) ClearOptions() {
	q.options = []option.Option{}
}

// AddValidationRule 添加校验规则
func (q *RadioQuestion) AddValidationRule(rule validation.ValidationRule) {
	q.ValidationAbility.AddValidationRule(rule)
}

// SetCalculationRule 设置计算规则
func (q *RadioQuestion) SetCalculationRule(rule *calculation.CalculationRule) {
	q.CalculationAbility.SetCalculationRule(rule)
}
