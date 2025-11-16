package validation

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/questionnaire"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/validation/concurrent"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/validation/sequential"
)

// ServiceFactory 验证服务工厂
type ServiceFactory struct {
	questionnaireService questionnaire.Service
}

// NewServiceFactory 创建验证服务工厂
func NewServiceFactory(questionnaireService questionnaire.Service) *ServiceFactory {
	return &ServiceFactory{
		questionnaireService: questionnaireService,
	}
}

// CreateSequentialService 创建串行验证服务
func (f *ServiceFactory) CreateSequentialService() Service {
	sequentialService := sequential.NewService(f.questionnaireService)
	return &sequentialServiceWrapper{
		service: sequentialService,
	}
}

// CreateConcurrentService 创建并发验证服务
func (f *ServiceFactory) CreateConcurrentService(maxConcurrency int) ServiceConcurrent {
	concurrentService := concurrent.NewService(f.questionnaireService, maxConcurrency)
	return &concurrentServiceWrapper{
		service:        concurrentService,
		maxConcurrency: maxConcurrency,
	}
}

// CreateConcurrentServiceWithAdapter 创建带适配器的并发验证服务
func (f *ServiceFactory) CreateConcurrentServiceWithAdapter(maxConcurrency int) Service {
	concurrentService := f.CreateConcurrentService(maxConcurrency)
	return NewServiceAdapter(concurrentService)
}

// sequentialServiceWrapper 串行服务包装器
type sequentialServiceWrapper struct {
	service sequential.Service
}

func (w *sequentialServiceWrapper) ValidateAnswersheet(ctx context.Context, req *ValidationRequest) error {
	return w.service.ValidateAnswersheet(ctx, req.ToSequentialRequest())
}

func (w *sequentialServiceWrapper) ValidateQuestionnaireCode(ctx context.Context, code string) error {
	return w.service.ValidateQuestionnaireCode(ctx, code)
}

// concurrentServiceWrapper 并发服务包装器
type concurrentServiceWrapper struct {
	service        concurrent.Service
	maxConcurrency int
}

func (w *concurrentServiceWrapper) ValidateAnswersheet(ctx context.Context, req *ValidationRequest) error {
	return w.service.ValidateAnswersheet(ctx, req.ToConcurrentRequest())
}

func (w *concurrentServiceWrapper) ValidateQuestionnaireCode(ctx context.Context, code string) error {
	return w.service.ValidateQuestionnaireCode(ctx, code)
}

func (w *concurrentServiceWrapper) GetMaxConcurrency() int {
	return w.maxConcurrency
}

func (w *concurrentServiceWrapper) SetMaxConcurrency(maxConcurrency int) {
	w.maxConcurrency = maxConcurrency
	// 注意：这里只更新包装器中的值，实际的并发服务可能需要重新创建
}

// NewServiceConcurrent 创建并发服务（兼容Container的调用方式）
func NewServiceConcurrent(questionnaireValidator QuestionnaireValidator, answerValidator AnswerValidatorConcurrent) ServiceConcurrent {
	// 这是一个简化的实现，主要是为了满足Container的接口需求
	// 实际实现可能需要更复杂的逻辑
	return &simpleConcurrentService{
		questionnaireValidator: questionnaireValidator,
		answerValidator:        answerValidator,
		maxConcurrency:         10, // 默认并发数
	}
}

// simpleConcurrentService 简单的并发服务实现
type simpleConcurrentService struct {
	questionnaireValidator QuestionnaireValidator
	answerValidator        AnswerValidatorConcurrent
	maxConcurrency         int
}

func (s *simpleConcurrentService) ValidateAnswersheet(ctx context.Context, req *ValidationRequest) error {
	// 简化实现：先验证问卷代码
	if err := s.ValidateQuestionnaireCode(ctx, req.QuestionnaireCode); err != nil {
		return err
	}
	// 这里应该有更完整的答卷验证逻辑
	return nil
}

func (s *simpleConcurrentService) ValidateQuestionnaireCode(ctx context.Context, code string) error {
	return s.questionnaireValidator.ValidateQuestionnaireCode(ctx, code)
}

func (s *simpleConcurrentService) GetMaxConcurrency() int {
	return s.maxConcurrency
}

func (s *simpleConcurrentService) SetMaxConcurrency(maxConcurrency int) {
	s.maxConcurrency = maxConcurrency
}
