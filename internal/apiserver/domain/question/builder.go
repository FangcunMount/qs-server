package question

import (
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question/calculation"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question/option"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question/validation"
)

// BuilderOption 构建器选项函数类型
type BuilderOption func(*QuestionBuilder)

// QuestionBuilder 问题构建器 - 配置容器
type QuestionBuilder struct {
	// 基础信息
	code         QuestionCode
	title        string
	tips         string
	questionType QuestionType

	// 特定属性
	placeholder string
	options     []option.Option

	// 能力配置
	validationRules []validation.ValidationRule
	calculationRule *calculation.CalculationRule
}

// QuestionFactory 问题工厂接口
type QuestionFactory interface {
	CreateFromBuilder(builder *QuestionBuilder) Question
}

// NewQuestionBuilder 创建新的问题构建器
func NewQuestionBuilder() *QuestionBuilder {
	return &QuestionBuilder{
		options:         make([]option.Option, 0),
		validationRules: make([]validation.ValidationRule, 0),
	}
}

// ================================
// With函数式选项模式
// ================================

// WithCode 设置问题编码
func WithCode(code QuestionCode) BuilderOption {
	return func(b *QuestionBuilder) {
		b.code = code
	}
}

// WithTitle 设置问题标题
func WithTitle(title string) BuilderOption {
	return func(b *QuestionBuilder) {
		b.title = title
	}
}

// WithTips 设置问题提示
func WithTips(tips string) BuilderOption {
	return func(b *QuestionBuilder) {
		b.tips = tips
	}
}

// WithQuestionType 设置问题类型
func WithQuestionType(questionType QuestionType) BuilderOption {
	return func(b *QuestionBuilder) {
		b.questionType = questionType
	}
}

// WithPlaceholder 设置占位符
func WithPlaceholder(placeholder string) BuilderOption {
	return func(b *QuestionBuilder) {
		b.placeholder = placeholder
	}
}

// WithOptions 设置选项列表
func WithOptions(options []option.Option) BuilderOption {
	return func(b *QuestionBuilder) {
		b.options = options
	}
}

// WithOption 添加单个选项
func WithOption(code, content string, score int) BuilderOption {
	return func(b *QuestionBuilder) {
		opt := option.NewOption(code, content, score)
		b.options = append(b.options, opt)
	}
}

// WithValidationRules 设置校验规则列表
func WithValidationRules(rules []validation.ValidationRule) BuilderOption {
	return func(b *QuestionBuilder) {
		b.validationRules = rules
	}
}

// WithValidationRule 添加单个校验规则
func WithValidationRule(ruleType validation.RuleType, targetValue string) BuilderOption {
	return func(b *QuestionBuilder) {
		rule := validation.NewValidationRule(ruleType, targetValue)
		b.validationRules = append(b.validationRules, rule)
	}
}

// WithCalculationRule 设置计算规则
func WithCalculationRule(formula calculation.FormulaType) BuilderOption {
	return func(b *QuestionBuilder) {
		b.calculationRule = calculation.NewCalculationRule(formula)
	}
}

// ================================
// 便捷的校验规则选项
// ================================

// WithRequired 设置必填
func WithRequired() BuilderOption {
	return WithValidationRule(validation.RuleTypeRequired, "true")
}

// WithMinLength 设置最小长度
func WithMinLength(length int) BuilderOption {
	return WithValidationRule(validation.RuleTypeMinLength, string(rune(length+'0')))
}

// WithMaxLength 设置最大长度
func WithMaxLength(length int) BuilderOption {
	return WithValidationRule(validation.RuleTypeMaxLength, string(rune(length+'0')))
}

// WithMinValue 设置最小值
func WithMinValue(value int) BuilderOption {
	return WithValidationRule(validation.RuleTypeMinValue, string(rune(value+'0')))
}

// WithMaxValue 设置最大值
func WithMaxValue(value int) BuilderOption {
	return WithValidationRule(validation.RuleTypeMaxValue, string(rune(value+'0')))
}

// ================================
// 链式调用方法
// ================================

func (b *QuestionBuilder) SetCode(code QuestionCode) *QuestionBuilder {
	b.code = code
	return b
}

func (b *QuestionBuilder) SetTitle(title string) *QuestionBuilder {
	b.title = title
	return b
}

func (b *QuestionBuilder) SetTips(tips string) *QuestionBuilder {
	b.tips = tips
	return b
}

func (b *QuestionBuilder) SetQuestionType(questionType QuestionType) *QuestionBuilder {
	b.questionType = questionType
	return b
}

func (b *QuestionBuilder) SetPlaceholder(placeholder string) *QuestionBuilder {
	b.placeholder = placeholder
	return b
}

func (b *QuestionBuilder) AddOption(code, content string, score int) *QuestionBuilder {
	opt := option.NewOption(code, content, score)
	b.options = append(b.options, opt)
	return b
}

func (b *QuestionBuilder) AddValidationRule(ruleType validation.RuleType, targetValue string) *QuestionBuilder {
	rule := validation.NewValidationRule(ruleType, targetValue)
	b.validationRules = append(b.validationRules, rule)
	return b
}

func (b *QuestionBuilder) SetCalculationRule(formula calculation.FormulaType) *QuestionBuilder {
	b.calculationRule = calculation.NewCalculationRule(formula)
	return b
}

// ================================
// 获取构建的配置信息
// ================================

func (b *QuestionBuilder) GetCode() QuestionCode {
	return b.code
}

func (b *QuestionBuilder) GetTitle() string {
	return b.title
}

func (b *QuestionBuilder) GetTips() string {
	return b.tips
}

func (b *QuestionBuilder) GetQuestionType() QuestionType {
	return b.questionType
}

func (b *QuestionBuilder) GetPlaceholder() string {
	return b.placeholder
}

func (b *QuestionBuilder) GetOptions() []option.Option {
	return b.options
}

func (b *QuestionBuilder) GetValidationRules() []validation.ValidationRule {
	return b.validationRules
}

func (b *QuestionBuilder) GetCalculationRule() *calculation.CalculationRule {
	return b.calculationRule
}

// ================================
// 便捷构建函数
// ================================

// BuildQuestionWithOptions 使用函数式选项创建问题构建器
func BuildQuestionWithOptions(opts ...BuilderOption) *QuestionBuilder {
	builder := NewQuestionBuilder()
	for _, opt := range opts {
		opt(builder)
	}
	return builder
}

// QuickBuildTextQuestion 快速创建文本问题构建器
func QuickBuildTextQuestion(code QuestionCode, title string, opts ...BuilderOption) *QuestionBuilder {
	allOpts := append([]BuilderOption{
		WithCode(code),
		WithTitle(title),
		WithQuestionType(QuestionTypeText),
	}, opts...)
	return BuildQuestionWithOptions(allOpts...)
}

// QuickBuildRadioQuestion 快速创建单选问题构建器
func QuickBuildRadioQuestion(code QuestionCode, title string, opts ...BuilderOption) *QuestionBuilder {
	allOpts := append([]BuilderOption{
		WithCode(code),
		WithTitle(title),
		WithQuestionType(QuestionTypeRadio),
	}, opts...)
	return BuildQuestionWithOptions(allOpts...)
}

// QuickBuildNumberQuestion 快速创建数字问题构建器
func QuickBuildNumberQuestion(code QuestionCode, title string, opts ...BuilderOption) *QuestionBuilder {
	allOpts := append([]BuilderOption{
		WithCode(code),
		WithTitle(title),
		WithQuestionType(QuestionTypeNumber),
	}, opts...)
	return BuildQuestionWithOptions(allOpts...)
}

// QuickBuildCheckboxQuestion 快速创建多选问题构建器
func QuickBuildCheckboxQuestion(code QuestionCode, title string, opts ...BuilderOption) *QuestionBuilder {
	allOpts := append([]BuilderOption{
		WithCode(code),
		WithTitle(title),
		WithQuestionType(QuestionTypeCheckbox),
	}, opts...)
	return BuildQuestionWithOptions(allOpts...)
}

// Build 通过工厂构建问题对象
func (b *QuestionBuilder) Build(factory QuestionFactory) Question {
	return factory.CreateFromBuilder(b)
}
