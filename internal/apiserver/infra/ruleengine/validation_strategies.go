package ruleengine

import (
	"fmt"
	"regexp"
	"strconv"
	"unicode/utf8"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/validation"
	ruleengineport "github.com/FangcunMount/qs-server/internal/apiserver/port/ruleengine"
)

type validationStrategy interface {
	Validate(value ruleengineport.ValidatableValue, rule validation.ValidationRule) error
	SupportRuleType() validation.RuleType
}

type validationStrategies map[validation.RuleType]validationStrategy

func newDefaultValidationStrategies() validationStrategies {
	strategies := validationStrategies{}
	strategies.Register(&requiredStrategy{})
	strategies.Register(&minLengthStrategy{})
	strategies.Register(&maxLengthStrategy{})
	strategies.Register(&minValueStrategy{})
	strategies.Register(&maxValueStrategy{})
	strategies.Register(&minSelectionsStrategy{})
	strategies.Register(&maxSelectionsStrategy{})
	strategies.Register(&patternStrategy{})
	return strategies
}

func (s validationStrategies) Register(strategy validationStrategy) {
	s[strategy.SupportRuleType()] = strategy
}

func (s validationStrategies) Get(ruleType validation.RuleType) validationStrategy {
	return s[ruleType]
}

type requiredStrategy struct{}

func (s *requiredStrategy) Validate(value ruleengineport.ValidatableValue, _ validation.ValidationRule) error {
	if value.IsEmpty() {
		return fmt.Errorf("该字段为必填项")
	}
	return nil
}

func (s *requiredStrategy) SupportRuleType() validation.RuleType {
	return validation.RuleTypeRequired
}

type minLengthStrategy struct{}

func (s *minLengthStrategy) Validate(value ruleengineport.ValidatableValue, rule validation.ValidationRule) error {
	if value.IsEmpty() {
		return nil
	}
	minLength, err := strconv.Atoi(rule.GetTargetValue())
	if err != nil {
		return fmt.Errorf("invalid min_length rule value: %s", rule.GetTargetValue())
	}
	if utf8.RuneCountInString(value.AsString()) < minLength {
		return fmt.Errorf("字符数不得少于 %d 个", minLength)
	}
	return nil
}

func (s *minLengthStrategy) SupportRuleType() validation.RuleType {
	return validation.RuleTypeMinLength
}

type maxLengthStrategy struct{}

func (s *maxLengthStrategy) Validate(value ruleengineport.ValidatableValue, rule validation.ValidationRule) error {
	if value.IsEmpty() {
		return nil
	}
	maxLength, err := strconv.Atoi(rule.GetTargetValue())
	if err != nil {
		return fmt.Errorf("invalid max_length rule value: %s", rule.GetTargetValue())
	}
	if utf8.RuneCountInString(value.AsString()) > maxLength {
		return fmt.Errorf("字符数不得超过 %d 个", maxLength)
	}
	return nil
}

func (s *maxLengthStrategy) SupportRuleType() validation.RuleType {
	return validation.RuleTypeMaxLength
}

type minValueStrategy struct{}

func (s *minValueStrategy) Validate(value ruleengineport.ValidatableValue, rule validation.ValidationRule) error {
	if value.IsEmpty() {
		return nil
	}
	minValue, err := strconv.ParseFloat(rule.GetTargetValue(), 64)
	if err != nil {
		return fmt.Errorf("invalid min_value rule value: %s", rule.GetTargetValue())
	}
	actualValue, err := value.AsNumber()
	if err != nil {
		return fmt.Errorf("无法将值转换为数字: %v", err)
	}
	if actualValue < minValue {
		return fmt.Errorf("值不得小于 %v", minValue)
	}
	return nil
}

func (s *minValueStrategy) SupportRuleType() validation.RuleType {
	return validation.RuleTypeMinValue
}

type maxValueStrategy struct{}

func (s *maxValueStrategy) Validate(value ruleengineport.ValidatableValue, rule validation.ValidationRule) error {
	if value.IsEmpty() {
		return nil
	}
	maxValue, err := strconv.ParseFloat(rule.GetTargetValue(), 64)
	if err != nil {
		return fmt.Errorf("invalid max_value rule value: %s", rule.GetTargetValue())
	}
	actualValue, err := value.AsNumber()
	if err != nil {
		return fmt.Errorf("无法将值转换为数字: %v", err)
	}
	if actualValue > maxValue {
		return fmt.Errorf("值不得大于 %v", maxValue)
	}
	return nil
}

func (s *maxValueStrategy) SupportRuleType() validation.RuleType {
	return validation.RuleTypeMaxValue
}

type minSelectionsStrategy struct{}

func (s *minSelectionsStrategy) Validate(value ruleengineport.ValidatableValue, rule validation.ValidationRule) error {
	if value.IsEmpty() {
		return nil
	}
	minSelections, err := strconv.Atoi(rule.GetTargetValue())
	if err != nil {
		return fmt.Errorf("invalid min_selections rule value: %s", rule.GetTargetValue())
	}
	if len(value.AsArray()) < minSelections {
		return fmt.Errorf("至少需要选择 %d 项", minSelections)
	}
	return nil
}

func (s *minSelectionsStrategy) SupportRuleType() validation.RuleType {
	return validation.RuleTypeMinSelections
}

type maxSelectionsStrategy struct{}

func (s *maxSelectionsStrategy) Validate(value ruleengineport.ValidatableValue, rule validation.ValidationRule) error {
	if value.IsEmpty() {
		return nil
	}
	maxSelections, err := strconv.Atoi(rule.GetTargetValue())
	if err != nil {
		return fmt.Errorf("invalid max_selections rule value: %s", rule.GetTargetValue())
	}
	if len(value.AsArray()) > maxSelections {
		return fmt.Errorf("最多只能选择 %d 项", maxSelections)
	}
	return nil
}

func (s *maxSelectionsStrategy) SupportRuleType() validation.RuleType {
	return validation.RuleTypeMaxSelections
}

type patternStrategy struct{}

func (s *patternStrategy) Validate(value ruleengineport.ValidatableValue, rule validation.ValidationRule) error {
	if value.IsEmpty() {
		return nil
	}
	pattern := rule.GetTargetValue()
	if pattern == "" {
		return fmt.Errorf("pattern rule requires a non-empty pattern")
	}
	regex, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("invalid pattern: %v", err)
	}
	if !regex.MatchString(value.AsString()) {
		return fmt.Errorf("输入格式不正确")
	}
	return nil
}

func (s *patternStrategy) SupportRuleType() validation.RuleType {
	return validation.RuleTypePattern
}
