package concurrent

import (
	"context"
	"fmt"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/questionnaire"
	"github.com/FangcunMount/qs-server/internal/collection-server/domain/answersheet"
)

// Service 并发验证服务接口
type Service interface {
	// ValidateAnswersheet 验证答卷
	ValidateAnswersheet(ctx context.Context, req *ValidationRequest) error

	// ValidateQuestionnaireCode 验证问卷代码
	ValidateQuestionnaireCode(ctx context.Context, code string) error
}

// ValidationRequest 验证请求
type ValidationRequest struct {
	QuestionnaireCode string                  `json:"questionnaire_code" validate:"required"`
	Title             string                  `json:"title" validate:"required"`
	TesteeInfo        *TesteeInfo             `json:"testee_info" validate:"required"`
	Answers           []*AnswerValidationItem `json:"answers" validate:"required"`
}

// TesteeInfo 测试者信息
type TesteeInfo struct {
	Name   string `json:"name" validate:"required"`
	Gender string `json:"gender,omitempty"`
	Age    *int   `json:"age,omitempty"`
	Email  string `json:"email,omitempty"`
	Phone  string `json:"phone,omitempty"`
}

// AnswerValidationItem 答案验证项
type AnswerValidationItem struct {
	QuestionCode string      `json:"question_code" validate:"required"`
	QuestionType string      `json:"question_type" validate:"required"`
	Value        interface{} `json:"value" validate:"required"`
}

// service 并发验证服务实现
type service struct {
	questionnaireService questionnaire.Service
	answersheetValidator *answersheet.Validator
	maxConcurrency       int
}

// NewService 创建并发验证服务
func NewService(questionnaireService questionnaire.Service, maxConcurrency int) Service {
	if maxConcurrency <= 0 {
		maxConcurrency = 10 // 默认并发数
	}

	return &service{
		questionnaireService: questionnaireService,
		answersheetValidator: answersheet.NewValidator(),
		maxConcurrency:       maxConcurrency,
	}
}

// ValidateAnswersheet 验证答卷
func (s *service) ValidateAnswersheet(ctx context.Context, req *ValidationRequest) error {
	if req == nil {
		return fmt.Errorf("validation request cannot be nil")
	}

	log.L(ctx).Infof("Validating answersheet for questionnaire: %s (concurrent, max concurrency: %d)", req.QuestionnaireCode, s.maxConcurrency)

	// 1. 验证问卷代码
	if err := s.ValidateQuestionnaireCode(ctx, req.QuestionnaireCode); err != nil {
		return fmt.Errorf("questionnaire code validation failed: %w", err)
	}

	// 2. 获取问卷信息用于验证
	questionnaireInfo, err := s.questionnaireService.GetQuestionnaireForValidation(ctx, req.QuestionnaireCode)
	if err != nil {
		return fmt.Errorf("failed to get questionnaire for validation: %w", err)
	}

	// 3. 转换为领域实体
	answersheetEntity := s.convertToAnswersheet(req)

	// 4. 使用专门的并发验证器
	validator := NewValidator(s.maxConcurrency)
	if err := validator.ValidateAnswersWithValidation(ctx, answersheetEntity, questionnaireInfo); err != nil {
		return fmt.Errorf("concurrent answersheet validation failed: %w", err)
	}

	log.L(ctx).Info("Concurrent validation completed successfully")
	return nil
}

// ValidateQuestionnaireCode 验证问卷代码
func (s *service) ValidateQuestionnaireCode(ctx context.Context, code string) error {
	return s.questionnaireService.ValidateQuestionnaireCode(ctx, code)
}

// GetMaxConcurrency 获取最大并发数
func (s *service) GetMaxConcurrency() int {
	return s.maxConcurrency
}

// SetMaxConcurrency 设置最大并发数
func (s *service) SetMaxConcurrency(maxConcurrency int) {
	if maxConcurrency > 0 {
		s.maxConcurrency = maxConcurrency
	}
}

// GetServiceInfo 获取服务信息
func (s *service) GetServiceInfo() map[string]interface{} {
	return map[string]interface{}{
		"service_type":    "concurrent",
		"max_concurrency": s.maxConcurrency,
		"domain_layer":    "answersheet",
	}
}

// convertToAnswersheet 将请求转换为答卷实体
func (s *service) convertToAnswersheet(req *ValidationRequest) *answersheet.SubmitRequest {
	answers := make([]*answersheet.Answer, 0, len(req.Answers))
	for _, answer := range req.Answers {
		answers = append(answers, &answersheet.Answer{
			QuestionCode: answer.QuestionCode,
			QuestionType: answer.QuestionType,
			Value:        answer.Value,
		})
	}

	// 处理Age字段，从指针转换为int
	age := 0
	if req.TesteeInfo.Age != nil {
		age = *req.TesteeInfo.Age
	}

	return &answersheet.SubmitRequest{
		QuestionnaireCode: req.QuestionnaireCode,
		Title:             req.Title,
		TesteeInfo: &answersheet.TesteeInfo{
			Name:   req.TesteeInfo.Name,
			Gender: req.TesteeInfo.Gender,
			Age:    age,
			Email:  req.TesteeInfo.Email,
			Phone:  req.TesteeInfo.Phone,
		},
		Answers: answers,
	}
}
