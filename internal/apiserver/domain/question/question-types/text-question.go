package question_types

import (
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question/validation"
)

// TextQuestion 文本问题
type TextQuestion struct {
	BaseQuestion
	validation.ValidationAbility

	placeholder string
}

// NewTextQuestion 创建文本问题
func NewTextQuestion(code question.QuestionCode, title string) *TextQuestion {
	return &TextQuestion{
		BaseQuestion: NewBaseQuestion(code, title, question.QuestionTypeText),
	}
}

// NewTextQuestionWithFields 使用字段创建文本问题
func NewTextQuestionWithFields(code question.QuestionCode, title string) *TextQuestion {
	q := &TextQuestion{}
	q.BaseQuestion = BaseQuestion{}
	return q
}

// GetPlaceholder 获取占位符
func (q *TextQuestion) GetPlaceholder() string {
	return q.placeholder
}

// GetValidationRules 获取校验规则 - 重写BaseQuestion的默认实现
func (q *TextQuestion) GetValidationRules() []validation.ValidationRule {
	return q.ValidationAbility.GetValidationRules()
}

func (q *TextQuestion) SetPlaceholder(placeholder string) {
	q.placeholder = placeholder
}

// AddValidationRule 添加校验规则
func (q *TextQuestion) AddValidationRule(rule validation.ValidationRule) {
	q.ValidationAbility.AddValidationRule(rule)
}
