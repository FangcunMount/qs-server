package answersheet

import (
	"context"
	"fmt"
	"strconv"

	"github.com/fangcun-mount/qs-server/internal/collection-server/domain/validation"
	"github.com/fangcun-mount/qs-server/internal/collection-server/domain/validation/rules"
)

// QuestionInfo 问题信息接口（避免循环导入）
type QuestionInfo interface {
	GetCode() string
	GetType() string
	GetOptions() []QuestionOption
	GetValidationRules() []QuestionValidationRule
}

// QuestionOption 问题选项接口
type QuestionOption interface {
	GetCode() string
	GetContent() string
	GetScore() int32
}

// QuestionValidationRule 问题验证规则接口
type QuestionValidationRule interface {
	GetRuleType() string
	GetTargetValue() string
	GetMessage() string
}

// QuestionnaireInfo 问卷信息接口
type QuestionnaireInfo interface {
	GetCode() string
	GetQuestions() []QuestionInfo
}

// Validator 答卷验证器
type Validator struct {
	validationValidator *validation.Validator
}

// NewValidator 创建答卷验证器
func NewValidator() *Validator {
	return &Validator{
		validationValidator: validation.NewValidator(),
	}
}

// ValidateTesteeInfo 验证测试者信息
func (v *Validator) ValidateTesteeInfo(info *TesteeInfo) error {
	if info == nil {
		return fmt.Errorf("testee info cannot be nil")
	}

	// 验证姓名
	if info.Name == "" {
		return fmt.Errorf("testee name cannot be empty")
	}
	if len(info.Name) > 50 {
		return fmt.Errorf("testee name cannot exceed 50 characters")
	}

	// 验证年龄
	if info.Age <= 0 || info.Age > 150 {
		return fmt.Errorf("invalid testee age: %d", info.Age)
	}

	// 验证性别
	if info.Gender == "" {
		return fmt.Errorf("testee gender cannot be empty")
	}
	validGenders := map[string]bool{
		"male":   true,
		"female": true,
		"other":  true,
	}
	if !validGenders[info.Gender] {
		return fmt.Errorf("invalid testee gender: %s", info.Gender)
	}

	// 验证邮箱（可选）
	if info.Email != "" {
		emailRule := validation.Email("邮箱格式不正确")
		if err := v.validationValidator.Validate(info.Email, emailRule); err != nil {
			return fmt.Errorf("invalid email: %w", err)
		}
	}

	// 验证手机号（可选）
	if info.Phone != "" {
		phoneRule := validation.Pattern("^1[3-9]\\d{9}$", "手机号格式不正确")
		if err := v.validationValidator.Validate(info.Phone, phoneRule); err != nil {
			return fmt.Errorf("invalid phone: %w", err)
		}
	}

	return nil
}

// ValidateAnswer 根据问题验证规则验证单个答案
func (v *Validator) ValidateAnswer(ctx context.Context, answer *Answer, question QuestionInfo) error {
	if answer == nil {
		return fmt.Errorf("answer cannot be nil")
	}

	if question == nil {
		return fmt.Errorf("question cannot be nil")
	}

	// 验证问题代码匹配
	if answer.QuestionCode != question.GetCode() {
		return fmt.Errorf("question code mismatch: expected %s, got %s", question.GetCode(), answer.QuestionCode)
	}

	// 验证问题类型匹配
	if answer.QuestionType != question.GetType() {
		return fmt.Errorf("question type mismatch: expected %s, got %s", question.GetType(), answer.QuestionType)
	}

	// 验证答案值不为空
	if answer.Value == nil {
		return fmt.Errorf("answer value cannot be nil")
	}

	// 根据问题的验证规则验证答案
	validationRules := question.GetValidationRules()
	if len(validationRules) > 0 {
		// 将问卷的验证规则转换为验证器的规则
		rules := v.convertValidationRules(validationRules)

		// 使用验证器验证答案
		errors := v.validationValidator.ValidateMultiple(answer.Value, rules)
		if len(errors) > 0 {
			// 返回第一个错误
			return fmt.Errorf("answer validation failed for question %s: %w", question.GetCode(), errors[0])
		}
	}

	// 根据问题类型进行额外验证
	if err := v.validateAnswerByType(answer, question); err != nil {
		return fmt.Errorf("type-specific validation failed for question %s: %w", question.GetCode(), err)
	}

	return nil
}

// validateAnswerByType 根据问题类型验证答案
func (v *Validator) validateAnswerByType(answer *Answer, question QuestionInfo) error {
	switch question.GetType() {
	case "text", "textarea":
		if str, ok := answer.Value.(string); !ok || str == "" {
			return fmt.Errorf("text answer value cannot be empty")
		}
	case "number", "rating":
		switch answer.Value.(type) {
		case int, int32, int64, float32, float64:
			// 数值类型有效
		default:
			return fmt.Errorf("number answer value must be numeric")
		}
	case "single_choice":
		// 单选题答案必须是字符串，且必须在选项列表中
		if str, ok := answer.Value.(string); !ok || str == "" {
			return fmt.Errorf("single choice answer value cannot be empty")
		} else {
			// 验证选项是否在问题选项中
			if !v.isValidChoice(str, question.GetOptions()) {
				return fmt.Errorf("invalid choice: %s", str)
			}
		}
	case "multiple_choice":
		// 多选题答案可以是字符串或字符串数组
		switch val := answer.Value.(type) {
		case string:
			if val == "" {
				return fmt.Errorf("multiple choice answer cannot be empty")
			}
			if !v.isValidChoice(val, question.GetOptions()) {
				return fmt.Errorf("invalid choice: %s", val)
			}
		case []string:
			if len(val) == 0 {
				return fmt.Errorf("multiple choice answer cannot be empty")
			}
			for _, choice := range val {
				if !v.isValidChoice(choice, question.GetOptions()) {
					return fmt.Errorf("invalid choice: %s", choice)
				}
			}
		default:
			return fmt.Errorf("multiple choice answer value must be string or string array")
		}
	}

	return nil
}

// isValidChoice 验证选择是否在选项列表中
func (v *Validator) isValidChoice(choice string, options []QuestionOption) bool {
	for _, option := range options {
		if option.GetCode() == choice {
			return true
		}
	}
	return false
}

// convertValidationRules 将问卷的验证规则转换为验证器的规则
func (v *Validator) convertValidationRules(protoRules []QuestionValidationRule) []*rules.BaseRule {
	rules := make([]*rules.BaseRule, 0, len(protoRules))

	for _, protoRule := range protoRules {
		rule := v.convertValidationRule(protoRule)
		if rule != nil {
			rules = append(rules, rule)
		}
	}

	return rules
}

// convertValidationRule 转换单个验证规则
func (v *Validator) convertValidationRule(protoRule QuestionValidationRule) *rules.BaseRule {
	switch protoRule.GetRuleType() {
	case "required":
		return validation.Required("此题为必答题")
	case "min_length":
		return validation.MinLength(parseInt(protoRule.GetTargetValue()), "答案长度不能少于指定字符数")
	case "max_length":
		return validation.MaxLength(parseInt(protoRule.GetTargetValue()), "答案长度不能超过指定字符数")
	case "min_value":
		return validation.MinValue(parseFloat(protoRule.GetTargetValue()), "答案不能小于指定值")
	case "max_value":
		return validation.MaxValue(parseFloat(protoRule.GetTargetValue()), "答案不能大于指定值")
	case "email":
		return validation.Email("邮箱格式不正确")
	case "pattern":
		return validation.Pattern(protoRule.GetTargetValue(), "格式不正确")
	default:
		// 未知的规则类型，跳过
		return nil
	}
}

// parseInt 解析字符串为整数
func parseInt(s string) int {
	if i, err := strconv.Atoi(s); err == nil {
		return i
	}
	return 0
}

// parseFloat 解析字符串为浮点数
func parseFloat(s string) float64 {
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}
	return 0.0
}

// ValidateAnswers 验证答案列表（需要问卷信息）
func (v *Validator) ValidateAnswers(ctx context.Context, answers []*Answer, questionnaire QuestionnaireInfo) error {
	if len(answers) == 0 {
		return fmt.Errorf("answers cannot be empty")
	}

	if questionnaire == nil {
		return fmt.Errorf("questionnaire cannot be nil")
	}

	// 创建问题映射，方便查找
	questionMap := make(map[string]QuestionInfo)
	for _, q := range questionnaire.GetQuestions() {
		questionMap[q.GetCode()] = q
	}

	// 验证答案唯一性（同一问题不能有多个答案）
	questionCodes := make(map[string]bool)

	for i, answer := range answers {
		// 查找对应的问题
		question, exists := questionMap[answer.QuestionCode]
		if !exists {
			return fmt.Errorf("question not found: %s", answer.QuestionCode)
		}

		// 检查重复答案
		if questionCodes[answer.QuestionCode] {
			return fmt.Errorf("duplicate answer for question: %s", answer.QuestionCode)
		}
		questionCodes[answer.QuestionCode] = true

		// 验证答案
		if err := v.ValidateAnswer(ctx, answer, question); err != nil {
			return fmt.Errorf("invalid answer at index %d: %w", i, err)
		}
	}

	return nil
}

// ValidateSubmitRequest 验证提交请求
func (v *Validator) ValidateSubmitRequest(ctx context.Context, req *SubmitRequest, questionnaire QuestionnaireInfo) error {
	if req == nil {
		return fmt.Errorf("submit request cannot be nil")
	}

	// 验证问卷代码
	if req.QuestionnaireCode == "" {
		return fmt.Errorf("questionnaire code cannot be empty")
	}

	// 验证标题
	if req.Title == "" {
		return fmt.Errorf("title cannot be empty")
	}
	if len(req.Title) > 200 {
		return fmt.Errorf("title cannot exceed 200 characters")
	}

	// 验证测试者信息
	if err := v.ValidateTesteeInfo(req.TesteeInfo); err != nil {
		return fmt.Errorf("invalid testee info: %w", err)
	}

	// 验证答案（需要问卷信息）
	if err := v.ValidateAnswers(ctx, req.Answers, questionnaire); err != nil {
		return fmt.Errorf("invalid answers: %w", err)
	}

	return nil
}
