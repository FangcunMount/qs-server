package types

import (
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question"
)

// QuestionFactory 问题工厂实现
type QuestionFactory struct{}

// NewQuestionFactory 创建问题工厂
func NewQuestionFactory() *QuestionFactory {
	return &QuestionFactory{}
}

// CreateFromBuilder 从构建器创建问题对象
func (f *QuestionFactory) CreateFromBuilder(builder *question.QuestionBuilder) question.Question {
	switch builder.GetQuestionType() {
	case question.QuestionTypeText:
		return f.createTextQuestion(builder)
	case question.QuestionTypeNumber:
		return f.createNumberQuestion(builder)
	case question.QuestionTypeRadio:
		return f.createRadioQuestion(builder)
	case question.QuestionTypeCheckbox:
		return f.createCheckboxQuestion(builder)
	case question.QuestionTypeSection:
		return f.createSectionQuestion(builder)
	case question.QuestionTypeTextarea:
		return f.createTextareaQuestion(builder)
	default:
		// 默认创建文本问题
		return f.createTextQuestion(builder)
	}
}

func (f *QuestionFactory) createTextQuestion(builder *question.QuestionBuilder) question.Question {
	q := NewTextQuestion(builder.GetCode(), builder.GetTitle())

	if builder.GetPlaceholder() != "" {
		q.SetPlaceholder(builder.GetPlaceholder())
	}

	for _, rule := range builder.GetValidationRules() {
		q.AddValidationRule(rule)
	}

	return q
}

func (f *QuestionFactory) createNumberQuestion(builder *question.QuestionBuilder) question.Question {
	q := NewNumberQuestion(builder.GetCode(), builder.GetTitle())

	if builder.GetPlaceholder() != "" {
		q.SetPlaceholder(builder.GetPlaceholder())
	}

	for _, rule := range builder.GetValidationRules() {
		q.AddValidationRule(rule)
	}

	return q
}

func (f *QuestionFactory) createRadioQuestion(builder *question.QuestionBuilder) question.Question {
	q := NewRadioQuestion(builder.GetCode(), builder.GetTitle())

	if len(builder.GetOptions()) > 0 {
		q.SetOptions(builder.GetOptions())
	}

	for _, rule := range builder.GetValidationRules() {
		q.AddValidationRule(rule)
	}

	if builder.GetCalculationRule() != nil {
		q.SetCalculationRule(builder.GetCalculationRule())
	}

	return q
}

func (f *QuestionFactory) createCheckboxQuestion(builder *question.QuestionBuilder) question.Question {
	q := NewCheckboxQuestion(builder.GetCode(), builder.GetTitle())

	if len(builder.GetOptions()) > 0 {
		q.SetOptions(builder.GetOptions())
	}

	for _, rule := range builder.GetValidationRules() {
		q.AddValidationRule(rule)
	}

	if builder.GetCalculationRule() != nil {
		q.SetCalculationRule(builder.GetCalculationRule())
	}

	return q
}

func (f *QuestionFactory) createSectionQuestion(builder *question.QuestionBuilder) question.Question {
	return NewSectionQuestion(builder.GetCode(), builder.GetTitle())
}

func (f *QuestionFactory) createTextareaQuestion(builder *question.QuestionBuilder) question.Question {
	q := NewTextQuestion(builder.GetCode(), builder.GetTitle())

	if builder.GetPlaceholder() != "" {
		q.SetPlaceholder(builder.GetPlaceholder())
	}

	for _, rule := range builder.GetValidationRules() {
		q.AddValidationRule(rule)
	}

	return q
}

// 便捷构建函数
func BuildQuestion(opts ...question.BuilderOption) question.Question {
	factory := NewQuestionFactory()
	return question.BuildQuestionWithOptions(opts...).Build(factory)
}
