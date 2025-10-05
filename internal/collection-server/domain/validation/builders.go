package validation

import (
	"fmt"

	"github.com/fangcun-mount/qs-server/internal/collection-server/domain/validation/rules"
)

// ValidationRuleBuilder 验证规则构建器
type ValidationRuleBuilder struct {
	rule *rules.BaseRule
}

// NewRule 创建新的验证规则构建器
func NewRule(name string) *ValidationRuleBuilder {
	return &ValidationRuleBuilder{
		rule: &rules.BaseRule{
			Name:   name,
			Params: make(map[string]interface{}),
		},
	}
}

// WithValue 设置规则值
func (b *ValidationRuleBuilder) WithValue(value interface{}) *ValidationRuleBuilder {
	b.rule.Value = value
	return b
}

// WithMessage 设置错误消息
func (b *ValidationRuleBuilder) WithMessage(message string) *ValidationRuleBuilder {
	b.rule.Message = message
	return b
}

// WithParam 添加参数
func (b *ValidationRuleBuilder) WithParam(key string, value interface{}) *ValidationRuleBuilder {
	b.rule.Params[key] = value
	return b
}

// Build 构建验证规则
func (b *ValidationRuleBuilder) Build() *rules.BaseRule {
	return b.rule
}

// 便捷的验证规则创建函数

// Required 创建必填验证规则
func Required(message string) *rules.BaseRule {
	if message == "" {
		message = "此字段为必填项"
	}
	return NewRule("required").WithMessage(message).Build()
}

// MaxValue 创建最大值验证规则
func MaxValue(maxValue float64, message string) *rules.BaseRule {
	if message == "" {
		message = fmt.Sprintf("值不能大于 %v", maxValue)
	}
	return NewRule("max_value").WithValue(maxValue).WithMessage(message).Build()
}

// MinValue 创建最小值验证规则
func MinValue(minValue float64, message string) *rules.BaseRule {
	if message == "" {
		message = fmt.Sprintf("值不能小于 %v", minValue)
	}
	return NewRule("min_value").WithValue(minValue).WithMessage(message).Build()
}

// MaxLength 创建最大长度验证规则
func MaxLength(maxLength int, message string) *rules.BaseRule {
	if message == "" {
		message = fmt.Sprintf("长度不能超过 %d 个字符", maxLength)
	}
	return NewRule("max_length").WithValue(maxLength).WithMessage(message).Build()
}

// MinLength 创建最小长度验证规则
func MinLength(minLength int, message string) *rules.BaseRule {
	if message == "" {
		message = fmt.Sprintf("长度不能少于 %d 个字符", minLength)
	}
	return NewRule("min_length").WithValue(minLength).WithMessage(message).Build()
}

// Pattern 创建正则表达式验证规则
func Pattern(pattern, message string) *rules.BaseRule {
	if message == "" {
		message = "格式不正确"
	}
	return NewRule("pattern").WithValue(pattern).WithMessage(message).Build()
}

// Email 创建邮箱验证规则
func Email(message string) *rules.BaseRule {
	if message == "" {
		message = "邮箱格式不正确"
	}
	return NewRule("email").WithMessage(message).Build()
}

// Phone 创建手机号验证规则
func Phone(message string) *rules.BaseRule {
	if message == "" {
		message = "手机号格式不正确"
	}
	return NewRule("phone").WithMessage(message).Build()
}

// OptionCode 创建选项代码验证规则
func OptionCode(allowedCodes []string, message string) *rules.BaseRule {
	if message == "" {
		message = "选择的选项不在允许范围内"
	}
	return NewRule("option_code").WithValue(allowedCodes).WithMessage(message).Build()
}

// Range 创建数值范围验证规则
func Range(minValue, maxValue float64, message string) *rules.BaseRule {
	if message == "" {
		message = fmt.Sprintf("答案必须在 %v 到 %v 之间", minValue, maxValue)
	}
	return NewRule("range").
		WithParam("min", minValue).
		WithParam("max", maxValue).
		WithMessage(message).
		Build()
}

// RangeRules 创建数值范围验证规则（返回多个规则）
func RangeRules(minValue, maxValue float64, message string) []*rules.BaseRule {
	if message == "" {
		message = fmt.Sprintf("答案必须在 %v 到 %v 之间", minValue, maxValue)
	}
	return []*rules.BaseRule{
		MinValue(minValue, ""),
		MaxValue(maxValue, message),
	}
}

// Length 创建长度范围验证规则
func Length(minLength, maxLength int, message string) []*rules.BaseRule {
	if message == "" {
		message = fmt.Sprintf("长度必须在 %d 到 %d 个字符之间", minLength, maxLength)
	}
	return []*rules.BaseRule{
		MinLength(minLength, ""),
		MaxLength(maxLength, message),
	}
}

// StringRules 字符串常用验证规则组合
type StringRules struct {
	Required  bool
	MinLength int
	MaxLength int
	Pattern   string
	Email     bool
}

// NewStringRules 创建字符串验证规则
func NewStringRules() *StringRules {
	return &StringRules{}
}

// SetRequired 设置必填
func (r *StringRules) SetRequired(required bool) *StringRules {
	r.Required = required
	return r
}

// SetMinLength 设置最小长度
func (r *StringRules) SetMinLength(minLength int) *StringRules {
	r.MinLength = minLength
	return r
}

// SetMaxLength 设置最大长度
func (r *StringRules) SetMaxLength(maxLength int) *StringRules {
	r.MaxLength = maxLength
	return r
}

// SetPattern 设置正则表达式
func (r *StringRules) SetPattern(pattern string) *StringRules {
	r.Pattern = pattern
	return r
}

// SetEmail 设置邮箱验证
func (r *StringRules) SetEmail(email bool) *StringRules {
	r.Email = email
	return r
}

// Build 构建验证规则列表
func (r *StringRules) Build() []*rules.BaseRule {
	var rules []*rules.BaseRule

	if r.Required {
		rules = append(rules, Required(""))
	}

	if r.MinLength > 0 {
		rules = append(rules, MinLength(r.MinLength, ""))
	}

	if r.MaxLength > 0 {
		rules = append(rules, MaxLength(r.MaxLength, ""))
	}

	if r.Pattern != "" {
		rules = append(rules, Pattern(r.Pattern, ""))
	}

	if r.Email {
		rules = append(rules, Email(""))
	}

	return rules
}

// NumberRules 数值常用验证规则组合
type NumberRules struct {
	Required bool
	MinValue float64
	MaxValue float64
}

// NewNumberRules 创建数值验证规则
func NewNumberRules() *NumberRules {
	return &NumberRules{}
}

// SetRequired 设置必填
func (r *NumberRules) SetRequired(required bool) *NumberRules {
	r.Required = required
	return r
}

// SetMinValue 设置最小值
func (r *NumberRules) SetMinValue(minValue float64) *NumberRules {
	r.MinValue = minValue
	return r
}

// SetMaxValue 设置最大值
func (r *NumberRules) SetMaxValue(maxValue float64) *NumberRules {
	r.MaxValue = maxValue
	return r
}

// SetRange 设置范围
func (r *NumberRules) SetRange(minValue, maxValue float64) *NumberRules {
	r.MinValue = minValue
	r.MaxValue = maxValue
	return r
}

// Build 构建验证规则列表
func (r *NumberRules) Build() []*rules.BaseRule {
	var rules []*rules.BaseRule

	if r.Required {
		rules = append(rules, Required(""))
	}

	if r.MinValue != 0 {
		rules = append(rules, MinValue(r.MinValue, ""))
	}

	if r.MaxValue != 0 {
		rules = append(rules, MaxValue(r.MaxValue, ""))
	}

	return rules
}
