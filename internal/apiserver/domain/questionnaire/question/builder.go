package question

import "github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire/question/ability"

// BuilderOption 构建器选项函数类型
type BuilderOption func(*QuestionBuilder)

// QuestionBuilder 问题构建器 - 纯配置容器
// 职责：收集和管理问题创建所需的所有配置参数
type QuestionBuilder struct {
	// 基础信息
	code         QuestionCode
	title        string
	tips         string
	questionType QuestionType

	// 特定属性
	placeholder string
	options     []Option

	// 能力配置
	validationRules []ability.ValidationRule
	calculationRule *ability.CalculationRule
}

// NewQuestionBuilder 创建新的问题构建器
func NewQuestionBuilder() *QuestionBuilder {
	return &QuestionBuilder{
		options:         make([]Option, 0),
		validationRules: make([]ability.ValidationRule, 0),
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
func WithOptions(options []Option) BuilderOption {
	return func(b *QuestionBuilder) {
		b.options = options
	}
}

// WithOption 添加单个选项
func WithOption(code, content string, score int) BuilderOption {
	return func(b *QuestionBuilder) {
		opt := NewOption(code, content, score)
		b.options = append(b.options, opt)
	}
}

// WithValidationRules 设置校验规则列表
func WithValidationRules(rules []ability.ValidationRule) BuilderOption {
	return func(b *QuestionBuilder) {
		b.validationRules = rules
	}
}

// WithValidationRule 添加单个校验规则
func WithValidationRule(ruleType ability.RuleType, targetValue string) BuilderOption {
	return func(b *QuestionBuilder) {
		rule := ability.NewValidationRule(ruleType, targetValue)
		b.validationRules = append(b.validationRules, rule)
	}
}

// WithCalculationRule 设置计算规则
func WithCalculationRule(formula ability.FormulaType) BuilderOption {
	return func(b *QuestionBuilder) {
		b.calculationRule = ability.NewCalculationRule(formula)
	}
}

// ================================
// 便捷的校验规则选项
// ================================

// WithRequired 设置必填
func WithRequired() BuilderOption {
	return WithValidationRule(ability.RuleTypeRequired, "true")
}

// WithMinLength 设置最小长度
func WithMinLength(length int) BuilderOption {
	return WithValidationRule(ability.RuleTypeMinLength, string(rune(length+'0')))
}

// WithMaxLength 设置最大长度
func WithMaxLength(length int) BuilderOption {
	return WithValidationRule(ability.RuleTypeMaxLength, string(rune(length+'0')))
}

// WithMinValue 设置最小值
func WithMinValue(value int) BuilderOption {
	return WithValidationRule(ability.RuleTypeMinValue, string(rune(value+'0')))
}

// WithMaxValue 设置最大值
func WithMaxValue(value int) BuilderOption {
	return WithValidationRule(ability.RuleTypeMaxValue, string(rune(value+'0')))
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
	opt := NewOption(code, content, score)
	b.options = append(b.options, opt)
	return b
}

func (b *QuestionBuilder) AddValidationRule(ruleType ability.RuleType, targetValue string) *QuestionBuilder {
	rule := ability.NewValidationRule(ruleType, targetValue)
	b.validationRules = append(b.validationRules, rule)
	return b
}

func (b *QuestionBuilder) SetCalculationRule(formula ability.FormulaType) *QuestionBuilder {
	b.calculationRule = ability.NewCalculationRule(formula)
	return b
}

// ================================
// 配置信息访问方法（只读）
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

func (b *QuestionBuilder) GetOptions() []Option {
	return b.options
}

func (b *QuestionBuilder) GetValidationRules() []ability.ValidationRule {
	return b.validationRules
}

func (b *QuestionBuilder) GetCalculationRule() *ability.CalculationRule {
	return b.calculationRule
}

// ================================
// 配置验证方法
// ================================

// IsValid 验证配置是否有效
func (b *QuestionBuilder) IsValid() bool {
	return b.code.Value() != "" && b.title != "" && b.questionType != ""
}

// GetValidationErrors 获取配置验证错误
func (b *QuestionBuilder) GetValidationErrors() []string {
	var errors []string

	if b.code.Value() == "" {
		errors = append(errors, "问题编码不能为空")
	}
	if b.title == "" {
		errors = append(errors, "问题标题不能为空")
	}
	if b.questionType == "" {
		errors = append(errors, "问题类型不能为空")
	}

	return errors
}

// ================================
// 便捷构建函数（仅创建Builder）
// ================================

// BuildQuestionConfig 使用函数式选项创建问题构建器
func BuildQuestionConfig(opts ...BuilderOption) *QuestionBuilder {
	builder := NewQuestionBuilder()
	for _, opt := range opts {
		opt(builder)
	}
	return builder
}
