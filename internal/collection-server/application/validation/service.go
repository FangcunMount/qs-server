package validation

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/validation/concurrent"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/validation/sequential"
	"github.com/FangcunMount/qs-server/internal/collection-server/infrastructure/grpc"
)

// Service 验证服务统一接口
type Service interface {
	// ValidateAnswersheet 验证答卷
	ValidateAnswersheet(ctx context.Context, req *ValidationRequest) error

	// ValidateQuestionnaireCode 验证问卷代码
	ValidateQuestionnaireCode(ctx context.Context, code string) error
}

// ServiceConcurrent 并发验证服务接口
type ServiceConcurrent interface {
	Service

	// GetMaxConcurrency 获取最大并发数
	GetMaxConcurrency() int

	// SetMaxConcurrency 设置最大并发数
	SetMaxConcurrency(maxConcurrency int)
}

// ValidationRequest 统一的验证请求结构
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

// ServiceAdapter 服务适配器，将并发服务适配为通用服务接口
type ServiceAdapter struct {
	concurrentService ServiceConcurrent
}

// NewServiceAdapter 创建服务适配器
func NewServiceAdapter(concurrentService ServiceConcurrent) Service {
	return &ServiceAdapter{
		concurrentService: concurrentService,
	}
}

// ValidateAnswersheet 验证答卷（通过并发服务实现）
func (a *ServiceAdapter) ValidateAnswersheet(ctx context.Context, req *ValidationRequest) error {
	return a.concurrentService.ValidateAnswersheet(ctx, req)
}

// ValidateQuestionnaireCode 验证问卷代码（通过并发服务实现）
func (a *ServiceAdapter) ValidateQuestionnaireCode(ctx context.Context, code string) error {
	return a.concurrentService.ValidateQuestionnaireCode(ctx, code)
}

// 转换函数，用于在不同包的请求结构间转换

// ToSequentialRequest 转换为串行验证请求
func (req *ValidationRequest) ToSequentialRequest() *sequential.ValidationRequest {
	sequentialAnswers := make([]*sequential.AnswerValidationItem, len(req.Answers))
	for i, answer := range req.Answers {
		sequentialAnswers[i] = &sequential.AnswerValidationItem{
			QuestionCode: answer.QuestionCode,
			QuestionType: answer.QuestionType,
			Value:        answer.Value,
		}
	}

	return &sequential.ValidationRequest{
		QuestionnaireCode: req.QuestionnaireCode,
		Title:             req.Title,
		TesteeInfo: &sequential.TesteeInfo{
			Name:   req.TesteeInfo.Name,
			Gender: req.TesteeInfo.Gender,
			Age:    req.TesteeInfo.Age,
			Email:  req.TesteeInfo.Email,
			Phone:  req.TesteeInfo.Phone,
		},
		Answers: sequentialAnswers,
	}
}

// ToConcurrentRequest 转换为并发验证请求
func (req *ValidationRequest) ToConcurrentRequest() *concurrent.ValidationRequest {
	concurrentAnswers := make([]*concurrent.AnswerValidationItem, len(req.Answers))
	for i, answer := range req.Answers {
		concurrentAnswers[i] = &concurrent.AnswerValidationItem{
			QuestionCode: answer.QuestionCode,
			QuestionType: answer.QuestionType,
			Value:        answer.Value,
		}
	}

	return &concurrent.ValidationRequest{
		QuestionnaireCode: req.QuestionnaireCode,
		Title:             req.Title,
		TesteeInfo: &concurrent.TesteeInfo{
			Name:   req.TesteeInfo.Name,
			Gender: req.TesteeInfo.Gender,
			Age:    req.TesteeInfo.Age,
			Email:  req.TesteeInfo.Email,
			Phone:  req.TesteeInfo.Phone,
		},
		Answers: concurrentAnswers,
	}
}

// 以下是Container期望的构造函数和类型（为了保持兼容性）

// QuestionnaireValidator 问卷验证器
type QuestionnaireValidator interface {
	ValidateQuestionnaireCode(ctx context.Context, code string) error
}

// questionnaireValidator 实现
type questionnaireValidator struct {
	client grpc.QuestionnaireClient
}

// NewQuestionnaireValidator 创建问卷验证器
func NewQuestionnaireValidator(client grpc.QuestionnaireClient) QuestionnaireValidator {
	return &questionnaireValidator{
		client: client,
	}
}

// ValidateQuestionnaireCode 验证问卷代码
func (v *questionnaireValidator) ValidateQuestionnaireCode(ctx context.Context, code string) error {
	// 这里可以调用gRPC客户端验证问卷代码
	// 简化实现，实际上应该调用真实的验证逻辑
	_, err := v.client.GetQuestionnaire(ctx, code)
	return err
}

// ValidationRuleFactory 验证规则工厂接口
type ValidationRuleFactory interface {
	// 这里可以添加规则创建方法
	CreateRequiredRule() interface{}
}

// defaultValidationRuleFactory 默认验证规则工厂
type defaultValidationRuleFactory struct{}

// NewDefaultValidationRuleFactory 创建默认验证规则工厂
func NewDefaultValidationRuleFactory() ValidationRuleFactory {
	return &defaultValidationRuleFactory{}
}

// CreateRequiredRule 创建必填规则
func (f *defaultValidationRuleFactory) CreateRequiredRule() interface{} {
	// 简化实现
	return struct{}{}
}

// AnswerValidatorConcurrent 并发答案验证器接口
type AnswerValidatorConcurrent interface {
	// 这里可以添加并发验证方法
}

// answerValidatorConcurrent 实现
type answerValidatorConcurrent struct {
	ruleFactory    ValidationRuleFactory
	maxConcurrency int
}

// NewAnswerValidatorConcurrent 创建并发答案验证器
func NewAnswerValidatorConcurrent(ruleFactory ValidationRuleFactory, maxConcurrency int) AnswerValidatorConcurrent {
	return &answerValidatorConcurrent{
		ruleFactory:    ruleFactory,
		maxConcurrency: maxConcurrency,
	}
}
