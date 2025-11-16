package types

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/questionnaire/question"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/questionnaire/question/ability"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/validation"
)

// 注册文本问题
func init() {
	question.RegisterQuestionFactory(question.QuestionTypeText, func(builder *question.QuestionBuilder) question.Question {
		// 创建文本问题
		q := newTextQuestion(builder.GetCode(), builder.GetTitle())

		// 设置占位符
		q.setPlaceholder(builder.GetPlaceholder())

		// 设置校验规则
		for _, rule := range builder.GetValidationRules() {
			q.addValidationRule(rule)
		}
		return q
	})
}

// TextQuestion 文本问题
type TextQuestion struct {
	BaseQuestion
	ability.ValidationAbility

	placeholder string
}

// NewTextQuestion 创建文本问题
func newTextQuestion(code meta.Code, title string) *TextQuestion {
	return &TextQuestion{
		BaseQuestion: NewBaseQuestion(code, title, question.QuestionTypeText),
	}
}

// setPlaceholder 设置占位符
func (q *TextQuestion) setPlaceholder(placeholder string) {
	q.placeholder = placeholder
}

// addValidationRule 添加校验规则
func (q *TextQuestion) addValidationRule(rule validation.ValidationRule) {
	q.ValidationAbility.AddValidationRule(rule)
}

// GetPlaceholder 获取占位符
func (q *TextQuestion) GetPlaceholder() string {
	return q.placeholder
}

// GetValidationRules 获取校验规则 - 重写BaseQuestion的默认实现
func (q *TextQuestion) GetValidationRules() []validation.ValidationRule {
	return q.ValidationAbility.GetValidationRules()
}
