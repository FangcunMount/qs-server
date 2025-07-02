package types

import (
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question"
)

// QuestionFactory 问题工厂接口
// 职责：专门负责根据配置创建具体的问题对象
type QuestionFactory interface {
	CreateFromBuilder(builder *QuestionBuilder) question.Question
}

// DefaultQuestionFactory 默认问题工厂实现
type DefaultQuestionFactory struct{}

// NewQuestionFactory 创建问题工厂
func NewQuestionFactory() QuestionFactory {
	return &DefaultQuestionFactory{}
}

// CreateFromBuilder 从构建器创建问题对象
func (f *DefaultQuestionFactory) CreateFromBuilder(builder *QuestionBuilder) question.Question {
	// 验证配置有效性
	if !builder.IsValid() {
		return nil // 或者返回错误
	}

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

// ================================
// 私有创建方法 - 各种题型的具体创建逻辑
// ================================

// createTextQuestion 创建文本问题
func (f *DefaultQuestionFactory) createTextQuestion(builder *QuestionBuilder) question.Question {
	q := NewTextQuestion(builder.GetCode(), builder.GetTitle())

	if builder.GetPlaceholder() != "" {
		q.SetPlaceholder(builder.GetPlaceholder())
	}

	for _, rule := range builder.GetValidationRules() {
		q.AddValidationRule(rule)
	}

	return q
}

// createNumberQuestion 创建数字问题
func (f *DefaultQuestionFactory) createNumberQuestion(builder *QuestionBuilder) question.Question {
	q := NewNumberQuestion(builder.GetCode(), builder.GetTitle())

	if builder.GetPlaceholder() != "" {
		q.SetPlaceholder(builder.GetPlaceholder())
	}

	for _, rule := range builder.GetValidationRules() {
		q.AddValidationRule(rule)
	}

	return q
}

// createRadioQuestion 创建单选问题
func (f *DefaultQuestionFactory) createRadioQuestion(builder *QuestionBuilder) question.Question {
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

// createCheckboxQuestion 创建多选问题
func (f *DefaultQuestionFactory) createCheckboxQuestion(builder *QuestionBuilder) question.Question {
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

// createSectionQuestion 创建分组问题
func (f *DefaultQuestionFactory) createSectionQuestion(builder *QuestionBuilder) question.Question {
	return NewSectionQuestion(builder.GetCode(), builder.GetTitle())
}

// createTextareaQuestion 创建文本域问题
func (f *DefaultQuestionFactory) createTextareaQuestion(builder *QuestionBuilder) question.Question {
	q := NewTextQuestion(builder.GetCode(), builder.GetTitle())

	if builder.GetPlaceholder() != "" {
		q.SetPlaceholder(builder.GetPlaceholder())
	}

	for _, rule := range builder.GetValidationRules() {
		q.AddValidationRule(rule)
	}

	return q
}

// ================================
// 便捷创建函数 - Factory 提供的一体化创建接口
// ================================

// CreateQuestion 直接从配置选项创建问题对象
func CreateQuestion(opts ...BuilderOption) question.Question {
	// 1. 创建配置
	builder := BuildQuestionConfig(opts...)

	// 2. 创建工厂
	factory := NewQuestionFactory()

	// 3. 创建对象
	return factory.CreateFromBuilder(builder)
}

// CreateTextQuestion 创建文本问题
func CreateTextQuestion(code question.QuestionCode, title string, opts ...BuilderOption) question.Question {
	allOpts := append([]BuilderOption{
		WithCode(code),
		WithTitle(title),
		WithQuestionType(question.QuestionTypeText),
	}, opts...)
	return CreateQuestion(allOpts...)
}

// CreateRadioQuestion 创建单选问题
func CreateRadioQuestion(code question.QuestionCode, title string, opts ...BuilderOption) question.Question {
	allOpts := append([]BuilderOption{
		WithCode(code),
		WithTitle(title),
		WithQuestionType(question.QuestionTypeRadio),
	}, opts...)
	return CreateQuestion(allOpts...)
}

// CreateNumberQuestion 创建数字问题
func CreateNumberQuestion(code question.QuestionCode, title string, opts ...BuilderOption) question.Question {
	allOpts := append([]BuilderOption{
		WithCode(code),
		WithTitle(title),
		WithQuestionType(question.QuestionTypeNumber),
	}, opts...)
	return CreateQuestion(allOpts...)
}

// CreateCheckboxQuestion 创建多选问题
func CreateCheckboxQuestion(code question.QuestionCode, title string, opts ...BuilderOption) question.Question {
	allOpts := append([]BuilderOption{
		WithCode(code),
		WithTitle(title),
		WithQuestionType(question.QuestionTypeCheckbox),
	}, opts...)
	return CreateQuestion(allOpts...)
}

// ================================
// 批量创建功能
// ================================

// CreateQuestionsFromBuilders 从多个构建器批量创建问题
func CreateQuestionsFromBuilders(builders []*QuestionBuilder) []question.Question {
	factory := NewQuestionFactory()
	questions := make([]question.Question, 0, len(builders))

	for _, builder := range builders {
		if q := factory.CreateFromBuilder(builder); q != nil {
			questions = append(questions, q)
		}
	}

	return questions
}

// CreateQuestionsFromConfigs 从多个配置批量创建问题
func CreateQuestionsFromConfigs(configs [][]BuilderOption) []question.Question {
	factory := NewQuestionFactory()
	questions := make([]question.Question, 0, len(configs))

	for _, opts := range configs {
		builder := BuildQuestionConfig(opts...)
		if q := factory.CreateFromBuilder(builder); q != nil {
			questions = append(questions, q)
		}
	}

	return questions
}
