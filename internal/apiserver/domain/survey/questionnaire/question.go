package questionnaire

import (
	"strconv"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/validation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/calculation"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// =========== 题型接口定义 ============

// Question 问题接口 - 统一所有题型的方法签名
type Question interface {
	// 基础方法
	GetType() QuestionType
	GetCode() meta.Code
	GetStem() string
	GetTips() string
	GetPlaceholder() string

	// 选项相关（若无则返回 nil）
	GetOptions() []Option

	// 校验规则相关
	GetValidationRules() []validation.ValidationRule

	// 计算规则相关
	GetCalculationRule() *calculation.CalculationRule
}

// HasOptions 带选项的问题接口
type HasOptions interface {
	Question
	GetOptions() []Option
}

// HasValidation 带校验的问题接口
type HasValidation interface {
	Question
	GetValidationRules() []validation.ValidationRule
}

// HasCalculation 带计算的问题接口
type HasCalculation interface {
	Question
	GetCalculationRule() *calculation.CalculationRule
}

// ============ 题型核心结构实现 ============

// QuestionCore 题型核心字段
type QuestionCore struct {
	code meta.Code
	typ  QuestionType
	stem string
	tips string
}

// NewQuestionCore 创建题型核心字段
func NewQuestionCore(code meta.Code, stem string, typ QuestionType) QuestionCore {
	return QuestionCore{
		code: code,
		stem: stem,
		typ:  typ,
	}
}

// Get*** 方法实现
func (q *QuestionCore) GetCode() meta.Code     { return q.code }
func (q *QuestionCore) GetStem() string        { return q.stem }
func (q *QuestionCore) GetType() QuestionType  { return q.typ }
func (q *QuestionCore) GetTips() string        { return q.tips }
func (q *QuestionCore) GetPlaceholder() string { return "" }
func (q *QuestionCore) GetOptions() []Option   { return nil }
func (q *QuestionCore) GetValidationRules() []validation.ValidationRule {
	return []validation.ValidationRule{}
}
func (q *QuestionCore) GetCalculationRule() *calculation.CalculationRule {
	return nil
}

// ============ 具体题型实现 ============

// ------------ 段落题 -----------
// SectionQuestion 段落题（纯展示，无需回答）
type SectionQuestion struct {
	QuestionCore
}

// ------------ 单选题 -----------
// RadioQuestion 单选题
type RadioQuestion struct {
	QuestionCore
	options         []Option
	validationRules []validation.ValidationRule
	calculationRule *calculation.CalculationRule
}

// GetOptions 获取选项
func (q *RadioQuestion) GetOptions() []Option {
	return q.options
}

// GetValidationRules 获取校验规则
func (q *RadioQuestion) GetValidationRules() []validation.ValidationRule {
	return q.validationRules
}

// GetCalculationRule 获取计算规则
func (q *RadioQuestion) GetCalculationRule() *calculation.CalculationRule {
	return q.calculationRule
}

// ------------ 多选题 -----------
// CheckboxQuestion 多选题
type CheckboxQuestion struct {
	QuestionCore
	options         []Option
	validationRules []validation.ValidationRule
	calculationRule *calculation.CalculationRule
}

// GetOptions 获取选项
func (q *CheckboxQuestion) GetOptions() []Option {
	return q.options
}

// GetValidationRules 获取校验规则
func (q *CheckboxQuestion) GetValidationRules() []validation.ValidationRule {
	return q.validationRules
}

// GetCalculationRule 获取计算规则
func (q *CheckboxQuestion) GetCalculationRule() *calculation.CalculationRule {
	return q.calculationRule
}

// ------------ 单行文本题 -----------
// TextQuestion 文本题（单行）
type TextQuestion struct {
	QuestionCore
	placeholder     string
	validationRules []validation.ValidationRule
}

// GetPlaceholder 获取占位符
func (q *TextQuestion) GetPlaceholder() string {
	return q.placeholder
}

// GetValidationRules 获取校验规则
func (q *TextQuestion) GetValidationRules() []validation.ValidationRule {
	return q.validationRules
}

// ------------ 多行文本题 -----------
// TextareaQuestion 文本域题（多行）
type TextareaQuestion struct {
	QuestionCore
	placeholder     string
	validationRules []validation.ValidationRule
}

// GetPlaceholder 获取占位符
func (q *TextareaQuestion) GetPlaceholder() string {
	return q.placeholder
}

// GetValidationRules 获取校验规则
func (q *TextareaQuestion) GetValidationRules() []validation.ValidationRule {
	return q.validationRules
}

// ------------ 数字题 -----------
// NumberQuestion 数字题
type NumberQuestion struct {
	QuestionCore
	placeholder     string
	validationRules []validation.ValidationRule
}

// GetPlaceholder 获取占位符
func (q *NumberQuestion) GetPlaceholder() string {
	return q.placeholder
}

// GetValidationRules 获取校验规则
func (q *NumberQuestion) GetValidationRules() []validation.ValidationRule {
	return q.validationRules
}

// ============ 题型工厂注册 ============

// init 注册所有题型工厂
func init() {
	// 注册段落题工厂
	RegisterQuestionFactory(TypeSection, newSectionQuestionFactory)

	// 注册单选题工厂
	RegisterQuestionFactory(TypeRadio, newRadioQuestionFactory)

	// 注册多选题工厂
	RegisterQuestionFactory(TypeCheckbox, newCheckboxQuestionFactory)

	// 注册文本题工厂
	RegisterQuestionFactory(TypeText, newTextQuestionFactory)

	// 注册文本域题工厂
	RegisterQuestionFactory(TypeTextarea, newTextareaQuestionFactory)

	// 注册数字题工厂
	RegisterQuestionFactory(TypeNumber, newNumberQuestionFactory)
}

// ============ 工厂函数实现 ============
// 每个工厂函数负责根据参数容器创建具体的 Question 实例

// 段落题工厂函数
func newSectionQuestionFactory(params *QuestionParams) (Question, error) {
	return &SectionQuestion{
		QuestionCore: params.GetCore(),
	}, nil
}

// 单选题工厂函数
func newRadioQuestionFactory(params *QuestionParams) (Question, error) {
	// 特定题型的参数校验
	if len(params.GetOptions()) == 0 {
		return nil, errors.WithCode(code.ErrOptionEmpty, "radio question options cannot be empty")
	}

	return &RadioQuestion{
		QuestionCore:    params.GetCore(),
		options:         params.GetOptions(),
		validationRules: params.GetValidationRules(),
		calculationRule: params.GetCalculationRule(),
	}, nil
}

// 多选题工厂函数
func newCheckboxQuestionFactory(params *QuestionParams) (Question, error) {
	// 特定题型的参数校验
	if len(params.GetOptions()) == 0 {
		return nil, errors.WithCode(code.ErrOptionEmpty, "checkbox question options cannot be empty")
	}

	return &CheckboxQuestion{
		QuestionCore:    params.GetCore(),
		options:         params.GetOptions(),
		validationRules: params.GetValidationRules(),
		calculationRule: params.GetCalculationRule(),
	}, nil
}

// 文本题工厂函数
func newTextQuestionFactory(params *QuestionParams) (Question, error) {
	return &TextQuestion{
		QuestionCore:    params.GetCore(),
		placeholder:     params.GetPlaceholder(),
		validationRules: params.GetValidationRules(),
	}, nil
}

// 文本域题工厂函数
func newTextareaQuestionFactory(params *QuestionParams) (Question, error) {
	return &TextareaQuestion{
		QuestionCore:    params.GetCore(),
		placeholder:     params.GetPlaceholder(),
		validationRules: params.GetValidationRules(),
	}, nil
}

// 数字题工厂函数
func newNumberQuestionFactory(params *QuestionParams) (Question, error) {
	return &NumberQuestion{
		QuestionCore:    params.GetCore(),
		placeholder:     params.GetPlaceholder(),
		validationRules: params.GetValidationRules(),
	}, nil
}

// ============ 题型参数容器及选项定义 ============

// QuestionParamsOption 统一的构造选项，作用于 QuestionParams。
type QuestionParamsOption func(*QuestionParams)

// QuestionParams 题型参数容器，纯数据容器，收集题目创建所需的所有字段。
// 注意：QuestionParams 只负责收集参数，不负责创建 Question 实例。
type QuestionParams struct {
	core            QuestionCore
	placeholder     string
	options         []Option
	validationRules []validation.ValidationRule
	calculationRule *calculation.CalculationRule
}

// NewQuestionParams 创建参数容器并应用选项
func NewQuestionParams(opts ...QuestionParamsOption) *QuestionParams {
	b := &QuestionParams{
		options:         make([]Option, 0),
		validationRules: make([]validation.ValidationRule, 0),
	}
	b.Apply(opts...)
	return b
}

// Apply 应用多个选项到参数容器
func (b *QuestionParams) Apply(opts ...QuestionParamsOption) {
	for _, opt := range opts {
		opt(b)
	}
}

// Validate 校验参数完整性
func (b *QuestionParams) Validate() error {
	if b.core.code.Value() == "" {
		return errors.New("question code is required")
	}
	if b.core.stem == "" {
		return errors.New("question stem is required")
	}
	if b.core.typ == "" {
		return errors.New("question type is required")
	}
	return nil
}

// Getters - 提供给工厂函数访问参数
func (b *QuestionParams) GetCore() QuestionCore                            { return b.core }
func (b *QuestionParams) GetPlaceholder() string                           { return b.placeholder }
func (b *QuestionParams) GetOptions() []Option                             { return b.options }
func (b *QuestionParams) GetValidationRules() []validation.ValidationRule  { return b.validationRules }
func (b *QuestionParams) GetCalculationRule() *calculation.CalculationRule { return b.calculationRule }

// 核心字段配置
func WithCode(code meta.Code) QuestionParamsOption {
	return func(b *QuestionParams) { b.core.code = code }
}
func WithStem(stem string) QuestionParamsOption {
	return func(b *QuestionParams) {
		b.core.stem = stem
	}
}
func WithTips(tips string) QuestionParamsOption {
	return func(b *QuestionParams) {
		b.core.tips = tips
	}
}
func WithQuestionType(QuestionType QuestionType) QuestionParamsOption {
	return func(b *QuestionParams) {
		b.core.typ = QuestionType
	}
}

// 题型特有字段配置
func WithPlaceholder(placeholder string) QuestionParamsOption {
	return func(b *QuestionParams) {
		b.placeholder = placeholder
	}
}
func WithOptions(options []Option) QuestionParamsOption {
	return func(b *QuestionParams) {
		b.options = options
	}
}
func WithOption(code, content string, score float64) QuestionParamsOption {
	return func(b *QuestionParams) {
		// 忽略错误，因为 WithOption 用于构建过程，最终会在 Validate 中统一检查
		if opt, err := NewOptionWithStringCode(code, content, score); err == nil {
			b.options = append(b.options, opt)
		}
	}
}
func WithValidationRules(rules []validation.ValidationRule) QuestionParamsOption {
	return func(b *QuestionParams) {
		b.validationRules = rules
	}
}
func WithValidationRule(ruleType validation.RuleType, targetValue string) QuestionParamsOption {
	return func(b *QuestionParams) {
		r := validation.NewValidationRule(ruleType, targetValue)
		b.validationRules = append(b.validationRules, r)
	}
}
func WithCalculationRule(formula calculation.FormulaType) QuestionParamsOption {
	return func(b *QuestionParams) {
		b.calculationRule = calculation.NewCalculationRule(formula, []string{})
	}
}

// 便捷的校验规则选项
func WithRequired() QuestionParamsOption {
	return WithValidationRule(validation.RuleTypeRequired, "true")
}
func WithMinLength(length int) QuestionParamsOption {
	return WithValidationRule(validation.RuleTypeMinLength, strconv.Itoa(length))
}
func WithMaxLength(length int) QuestionParamsOption {
	return WithValidationRule(validation.RuleTypeMaxLength, strconv.Itoa(length))
}
func WithMinValue(value int) QuestionParamsOption {
	return WithValidationRule(validation.RuleTypeMinValue, strconv.Itoa(value))
}
func WithMaxValue(value int) QuestionParamsOption {
	return WithValidationRule(validation.RuleTypeMaxValue, strconv.Itoa(value))
}
