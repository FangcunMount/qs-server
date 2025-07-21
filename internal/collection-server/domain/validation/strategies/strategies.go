package strategies

import (
	"fmt"
	"regexp"

	"github.com/yshujie/questionnaire-scale/internal/collection-server/domain/validation/rules"
)

// RequiredStrategy 必填验证策略
type RequiredStrategy struct {
	BaseStrategy
}

// NewRequiredStrategy 创建必填验证策略
func NewRequiredStrategy() *RequiredStrategy {
	return &RequiredStrategy{
		BaseStrategy: BaseStrategy{Name: "required"},
	}
}

// Validate 验证必填
func (s *RequiredStrategy) Validate(value interface{}, rule *rules.BaseRule) error {
	requiredRule := rules.NewRequiredRule(rule.Message)
	return requiredRule.Validate(value)
}

// MinValueStrategy 最小值验证策略
type MinValueStrategy struct {
	BaseStrategy
}

// NewMinValueStrategy 创建最小值验证策略
func NewMinValueStrategy() *MinValueStrategy {
	return &MinValueStrategy{
		BaseStrategy: BaseStrategy{Name: "min_value"},
	}
}

// Validate 验证最小值
func (s *MinValueStrategy) Validate(value interface{}, rule *rules.BaseRule) error {
	minValueRule := rules.NewMinValueRule(rule.Value, rule.Message)
	return minValueRule.Validate(value)
}

// MaxValueStrategy 最大值验证策略
type MaxValueStrategy struct {
	BaseStrategy
}

// NewMaxValueStrategy 创建最大值验证策略
func NewMaxValueStrategy() *MaxValueStrategy {
	return &MaxValueStrategy{
		BaseStrategy: BaseStrategy{Name: "max_value"},
	}
}

// Validate 验证最大值
func (s *MaxValueStrategy) Validate(value interface{}, rule *rules.BaseRule) error {
	maxValueRule := rules.NewMaxValueRule(rule.Value, rule.Message)
	return maxValueRule.Validate(value)
}

// MinLengthStrategy 最小长度验证策略
type MinLengthStrategy struct {
	BaseStrategy
}

// NewMinLengthStrategy 创建最小长度验证策略
func NewMinLengthStrategy() *MinLengthStrategy {
	return &MinLengthStrategy{
		BaseStrategy: BaseStrategy{Name: "min_length"},
	}
}

// Validate 验证最小长度
func (s *MinLengthStrategy) Validate(value interface{}, rule *rules.BaseRule) error {
	minLengthRule := rules.NewMinLengthRule(rule.Value, rule.Message)
	return minLengthRule.Validate(value)
}

// MaxLengthStrategy 最大长度验证策略
type MaxLengthStrategy struct {
	BaseStrategy
}

// NewMaxLengthStrategy 创建最大长度验证策略
func NewMaxLengthStrategy() *MaxLengthStrategy {
	return &MaxLengthStrategy{
		BaseStrategy: BaseStrategy{Name: "max_length"},
	}
}

// Validate 验证最大长度
func (s *MaxLengthStrategy) Validate(value interface{}, rule *rules.BaseRule) error {
	maxLengthRule := rules.NewMaxLengthRule(rule.Value, rule.Message)
	return maxLengthRule.Validate(value)
}

// PatternStrategy 正则表达式验证策略
type PatternStrategy struct {
	BaseStrategy
}

// NewPatternStrategy 创建正则表达式验证策略
func NewPatternStrategy() *PatternStrategy {
	return &PatternStrategy{
		BaseStrategy: BaseStrategy{Name: "pattern"},
	}
}

// Validate 验证正则表达式
func (s *PatternStrategy) Validate(value interface{}, rule *rules.BaseRule) error {
	if value == nil {
		return nil // 空值由 required 规则处理
	}

	pattern, ok := rule.Value.(string)
	if !ok {
		return fmt.Errorf("正则表达式必须是字符串")
	}

	str, ok := value.(string)
	if !ok {
		return rules.NewValidationError("", "正则表达式验证只支持字符串类型", value, s.GetStrategyName())
	}

	matched, err := regexp.MatchString(pattern, str)
	if err != nil {
		return fmt.Errorf("正则表达式错误: %w", err)
	}

	if !matched {
		message := rule.Message
		if message == "" {
			message = fmt.Sprintf("格式不正确，必须匹配正则表达式: %s", pattern)
		}
		return rules.NewValidationError("", message, value, s.GetStrategyName())
	}

	return nil
}

// EmailStrategy 邮箱验证策略
type EmailStrategy struct {
	BaseStrategy
}

// NewEmailStrategy 创建邮箱验证策略
func NewEmailStrategy() *EmailStrategy {
	return &EmailStrategy{
		BaseStrategy: BaseStrategy{Name: "email"},
	}
}

// Validate 验证邮箱
func (s *EmailStrategy) Validate(value interface{}, rule *rules.BaseRule) error {
	if value == nil {
		return nil // 空值由 required 规则处理
	}

	str, ok := value.(string)
	if !ok {
		return rules.NewValidationError("", "邮箱验证只支持字符串类型", value, s.GetStrategyName())
	}

	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	if !emailRegex.MatchString(str) {
		message := rule.Message
		if message == "" {
			message = "邮箱格式不正确"
		}
		return rules.NewValidationError("", message, value, s.GetStrategyName())
	}

	return nil
}
