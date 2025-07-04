package question_types

import (
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question/validation"
)

// NumberQuestion 数字问题
type NumberQuestion struct {
	BaseQuestion
	validation.ValidationAbility

	placeholder string
}

// 注册数字问题
func init() {
	RegisterQuestionFactory(question.QuestionTypeNumber, func(builder *QuestionBuilder) question.Question {
		// 创建数字问题
		q := newNumberQuestion(builder.GetCode(), builder.GetTitle())

		// 设置占位符
		q.setPlaceholder(builder.GetPlaceholder())

		// 设置校验规则
		for _, rule := range builder.GetValidationRules() {
			q.addValidationRule(rule)
		}
		return q
	})
}

// newNumberQuestion 创建数字问题
func newNumberQuestion(code question.QuestionCode, title string) *NumberQuestion {
	return &NumberQuestion{
		BaseQuestion: NewBaseQuestion(code, title, question.QuestionTypeNumber),
	}
}

// setPlaceholder 设置占位符
func (q *NumberQuestion) setPlaceholder(placeholder string) {
	q.placeholder = placeholder
}

// addValidationRule 添加校验规则
func (q *NumberQuestion) addValidationRule(rule validation.ValidationRule) {
	q.ValidationAbility.AddValidationRule(rule)
}

// GetPlaceholder 获取占位符
func (q *NumberQuestion) GetPlaceholder() string {
	return q.placeholder
}

// GetValidationRules 获取校验规则 - 重写BaseQuestion的默认实现
func (q *NumberQuestion) GetValidationRules() []validation.ValidationRule {
	return q.ValidationAbility.GetValidationRules()
}
