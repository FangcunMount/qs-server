package validation

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/yshujie/questionnaire-scale/pkg/log"
)

// Service 校验服务接口
type Service interface {
	// ValidateAnswersheet 校验答卷
	ValidateAnswersheet(ctx context.Context, answersheet *AnswersheetValidationRequest) error
	// ValidateQuestionnaireCode 校验问卷代码
	ValidateQuestionnaireCode(ctx context.Context, code string) error
}

// service 校验服务实现
type service struct{}

// NewService 创建新的校验服务
func NewService() Service {
	return &service{}
}

// AnswersheetValidationRequest 答卷校验请求
type AnswersheetValidationRequest struct {
	QuestionnaireCode string                 `json:"questionnaire_code"`
	Answers           []AnswerValidationItem `json:"answers"`
	TesteeInfo        TesteeInfo             `json:"testee_info"`
}

// AnswerValidationItem 答案校验项
type AnswerValidationItem struct {
	QuestionID string      `json:"question_id"`
	Value      interface{} `json:"value"`
}

// TesteeInfo 测试者信息
type TesteeInfo struct {
	Name   string `json:"name"`
	Age    int    `json:"age"`
	Gender string `json:"gender"`
	Email  string `json:"email"`
	Phone  string `json:"phone"`
}

// ValidateAnswersheet 校验答卷
func (s *service) ValidateAnswersheet(ctx context.Context, req *AnswersheetValidationRequest) error {
	log.L(ctx).Infof("Validating answersheet for questionnaire: %s", req.QuestionnaireCode)

	// 1. 校验问卷代码
	if err := s.ValidateQuestionnaireCode(ctx, req.QuestionnaireCode); err != nil {
		return fmt.Errorf("invalid questionnaire code: %w", err)
	}

	// 2. 校验测试者信息
	if err := s.validateTesteeInfo(ctx, req.TesteeInfo); err != nil {
		return fmt.Errorf("invalid testee info: %w", err)
	}

	// 3. 校验答案
	if err := s.validateAnswers(ctx, req.Answers); err != nil {
		return fmt.Errorf("invalid answers: %w", err)
	}

	log.L(ctx).Info("Answersheet validation passed")
	return nil
}

// ValidateQuestionnaireCode 校验问卷代码
func (s *service) ValidateQuestionnaireCode(ctx context.Context, code string) error {
	if code == "" {
		return fmt.Errorf("questionnaire code cannot be empty")
	}

	// 校验代码格式：只允许字母、数字、下划线和连字符
	if !isValidCode(code) {
		return fmt.Errorf("invalid questionnaire code format: %s", code)
	}

	// 校验代码长度
	if len(code) < 3 || len(code) > 50 {
		return fmt.Errorf("questionnaire code length must be between 3 and 50 characters")
	}

	return nil
}

// validateTesteeInfo 校验测试者信息
func (s *service) validateTesteeInfo(ctx context.Context, info TesteeInfo) error {
	// 校验姓名
	if strings.TrimSpace(info.Name) == "" {
		return fmt.Errorf("testee name cannot be empty")
	}

	if len(info.Name) > 100 {
		return fmt.Errorf("testee name too long")
	}

	// 校验年龄
	if info.Age < 0 || info.Age > 150 {
		return fmt.Errorf("invalid age: %d", info.Age)
	}

	// 校验性别
	if info.Gender != "" && !isValidGender(info.Gender) {
		return fmt.Errorf("invalid gender: %s", info.Gender)
	}

	// 校验邮箱
	if info.Email != "" && !isValidEmail(info.Email) {
		return fmt.Errorf("invalid email format: %s", info.Email)
	}

	// 校验手机号
	if info.Phone != "" && !isValidPhone(info.Phone) {
		return fmt.Errorf("invalid phone format: %s", info.Phone)
	}

	return nil
}

// validateAnswers 校验答案
func (s *service) validateAnswers(ctx context.Context, answers []AnswerValidationItem) error {
	if len(answers) == 0 {
		return fmt.Errorf("answers cannot be empty")
	}

	// 校验每个答案
	for i, answer := range answers {
		if err := s.validateAnswer(ctx, answer); err != nil {
			return fmt.Errorf("invalid answer at index %d: %w", i, err)
		}
	}

	return nil
}

// validateAnswer 校验单个答案
func (s *service) validateAnswer(ctx context.Context, answer AnswerValidationItem) error {
	// 校验问题ID
	if answer.QuestionID == "" {
		return fmt.Errorf("question ID cannot be empty")
	}

	// 校验答案值
	if answer.Value == nil {
		return fmt.Errorf("answer value cannot be nil")
	}

	// 校验答案值类型
	if err := s.validateAnswerValue(ctx, answer.Value); err != nil {
		return fmt.Errorf("invalid answer value for question %s: %w", answer.QuestionID, err)
	}

	return nil
}

// validateAnswerValue 校验答案值
func (s *service) validateAnswerValue(ctx context.Context, value interface{}) error {
	if value == nil {
		return fmt.Errorf("value cannot be nil")
	}

	// 根据值类型进行不同的校验
	switch v := value.(type) {
	case string:
		if len(v) > 10000 { // 限制文本长度
			return fmt.Errorf("text answer too long")
		}
	case int, int32, int64, float32, float64:
		// 数值类型校验
		return nil
	case []interface{}:
		// 数组类型校验
		if len(v) > 100 { // 限制数组长度
			return fmt.Errorf("array answer too long")
		}
	case map[string]interface{}:
		// 对象类型校验
		if len(v) > 50 { // 限制对象字段数
			return fmt.Errorf("object answer too complex")
		}
	default:
		// 其他类型校验
		log.L(ctx).Warnf("Unknown answer value type: %v", reflect.TypeOf(value))
	}

	return nil
}

// isValidCode 检查代码格式是否有效
func isValidCode(code string) bool {
	for _, char := range code {
		if !((char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '_' || char == '-') {
			return false
		}
	}
	return true
}

// isValidGender 检查性别是否有效
func isValidGender(gender string) bool {
	validGenders := []string{"male", "female", "other", "男", "女", "其他"}
	for _, valid := range validGenders {
		if strings.ToLower(gender) == strings.ToLower(valid) {
			return true
		}
	}
	return false
}

// isValidEmail 检查邮箱格式是否有效
func isValidEmail(email string) bool {
	// 简单的邮箱格式校验
	return strings.Contains(email, "@") && strings.Contains(email, ".")
}

// isValidPhone 检查手机号格式是否有效
func isValidPhone(phone string) bool {
	// 简单的手机号格式校验：只允许数字、空格、连字符和加号
	for _, char := range phone {
		if !((char >= '0' && char <= '9') || char == ' ' || char == '-' || char == '+') {
			return false
		}
	}
	return len(phone) >= 10 && len(phone) <= 20
}
