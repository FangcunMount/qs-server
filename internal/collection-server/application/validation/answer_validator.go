package validation

import (
	"context"
	"fmt"
	"strconv"

	questionnairepb "github.com/yshujie/questionnaire-scale/internal/apiserver/interface/grpc/proto/questionnaire"
	"github.com/yshujie/questionnaire-scale/internal/collection-server/domain/validation"
)

// ValidationRuleFactory 验证规则工厂接口
type ValidationRuleFactory interface {
	CreateValidationRules(question *questionnairepb.Question) []*validation.ValidationRule
}

// AnswerValidator 答案验证器
type AnswerValidator struct {
	validator   *validation.Validator
	ruleFactory ValidationRuleFactory
}

// NewAnswerValidator 创建答案验证器
func NewAnswerValidator(ruleFactory ValidationRuleFactory) *AnswerValidator {
	return &AnswerValidator{
		validator:   validation.NewValidator(),
		ruleFactory: ruleFactory,
	}
}

// ValidateAnswers 验证答案列表
func (v *AnswerValidator) ValidateAnswers(ctx context.Context, answers []AnswerValidationItem, questionnaire *questionnairepb.Questionnaire) error {
	if len(answers) == 0 {
		return fmt.Errorf("answers cannot be empty")
	}

	// 创建问题映射，方便查找
	questionMap := make(map[string]*questionnairepb.Question)
	for _, q := range questionnaire.Questions {
		questionMap[q.Code] = q
	}

	// 校验每个答案
	for i, answer := range answers {
		// 查找对应的问题
		question, exists := questionMap[answer.QuestionID]
		if !exists {
			return fmt.Errorf("question not found: %s", answer.QuestionID)
		}

		// 根据问题配置生成验证规则并校验
		if err := v.validateSingleAnswer(ctx, answer, question); err != nil {
			return fmt.Errorf("invalid answer at index %d (question %s): %w", i, answer.QuestionID, err)
		}
	}

	return nil
}

// validateSingleAnswer 验证单个答案
func (v *AnswerValidator) validateSingleAnswer(ctx context.Context, answer AnswerValidationItem, question *questionnairepb.Question) error {
	// 使用工厂生成验证规则
	rules := v.ruleFactory.CreateValidationRules(question)

	// 使用验证器校验答案
	errors := v.validator.ValidateMultiple(answer.Value, rules)
	if len(errors) > 0 {
		// 返回第一个错误
		return fmt.Errorf("validation failed: %s", errors[0].Error())
	}

	return nil
}

// DefaultValidationRuleFactory 默认验证规则工厂
type DefaultValidationRuleFactory struct{}

// NewDefaultValidationRuleFactory 创建默认验证规则工厂
func NewDefaultValidationRuleFactory() *DefaultValidationRuleFactory {
	return &DefaultValidationRuleFactory{}
}

// CreateValidationRules 根据问题配置生成验证规则
func (f *DefaultValidationRuleFactory) CreateValidationRules(question *questionnairepb.Question) []*validation.ValidationRule {
	var rules []*validation.ValidationRule

	// 处理问卷中配置的验证规则
	for _, protoRule := range question.ValidationRules {
		rule := f.convertProtoValidationRule(protoRule, question)
		if rule != nil {
			rules = append(rules, rule)
		}
	}

	return rules
}

// convertProtoValidationRule 转换 protobuf 验证规则为领域验证规则
func (f *DefaultValidationRuleFactory) convertProtoValidationRule(protoRule *questionnairepb.ValidationRule, question *questionnairepb.Question) *validation.ValidationRule {
	switch protoRule.RuleType {
	case "required":
		return validation.Required("此题为必答题")

	case "min_length":
		if length, err := strconv.Atoi(protoRule.TargetValue); err == nil {
			return validation.MinLength(length, fmt.Sprintf("答案长度不能少于%d个字符", length))
		}

	case "max_length":
		if length, err := strconv.Atoi(protoRule.TargetValue); err == nil {
			return validation.MaxLength(length, fmt.Sprintf("答案长度不能超过%d个字符", length))
		}

	case "min_value":
		if value, err := strconv.ParseFloat(protoRule.TargetValue, 64); err == nil {
			return validation.MinValue(value, fmt.Sprintf("答案不能小于%v", value))
		}

	case "max_value":
		if value, err := strconv.ParseFloat(protoRule.TargetValue, 64); err == nil {
			return validation.MaxValue(value, fmt.Sprintf("答案不能大于%v", value))
		}

	case "min_selections":
		if count, err := strconv.Atoi(protoRule.TargetValue); err == nil {
			// 对于多选题，验证最少选择数量
			return validation.MinValue(float64(count), fmt.Sprintf("至少需要选择%d个选项", count))
		}

	case "max_selections":
		if count, err := strconv.Atoi(protoRule.TargetValue); err == nil {
			// 对于多选题，验证最多选择数量
			return validation.MaxValue(float64(count), fmt.Sprintf("最多只能选择%d个选项", count))
		}

	case "option_code":
		// 选项代码验证：从问题选项中提取允许的代码
		if len(question.Options) > 0 {
			allowedCodes := make([]string, 0, len(question.Options))
			for _, option := range question.Options {
				allowedCodes = append(allowedCodes, option.Code)
			}
			return validation.OptionCode(allowedCodes, "选择的选项不在允许范围内")
		}

	case "email":
		return validation.Email("邮箱格式不正确")

	case "phone":
		return validation.Phone("手机号格式不正确")

	case "pattern":
		// 正则表达式验证
		return validation.Pattern(protoRule.TargetValue, "格式不正确")
	}

	return nil
}
