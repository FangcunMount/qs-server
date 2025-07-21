package validation

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/collection-server/application/questionnaire"
	"github.com/yshujie/questionnaire-scale/internal/collection-server/application/validation/concurrent"
	"github.com/yshujie/questionnaire-scale/internal/collection-server/application/validation/sequential"
)

// ValidationStrategy 验证策略
type ValidationStrategy string

const (
	SequentialStrategy ValidationStrategy = "sequential"
	ConcurrentStrategy ValidationStrategy = "concurrent"
)

// ValidationConfig 验证配置
type ValidationConfig struct {
	Strategy           ValidationStrategy `json:"strategy"`
	MaxConcurrency     int                `json:"max_concurrency"`
	ValidationTimeout  int                `json:"validation_timeout"`
	EnableDetailedLogs bool               `json:"enable_detailed_logs"`
}

// DefaultValidationConfig 默认验证配置
func DefaultValidationConfig() *ValidationConfig {
	return &ValidationConfig{
		Strategy:           SequentialStrategy,
		MaxConcurrency:     10,
		ValidationTimeout:  30,
		EnableDetailedLogs: false,
	}
}

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

// CreateService 创建验证服务
func (f *ServiceFactory) CreateService(strategy ValidationStrategy) Service {
	switch strategy {
	case ConcurrentStrategy:
		concurrentService := concurrent.NewService(f.questionnaireService, 10) // 默认并发数
		return &ConcurrentAdapter{service: concurrentService}
	case SequentialStrategy:
		fallthrough
	default:
		sequentialService := sequential.NewService(f.questionnaireService)
		return &SequentialAdapter{service: sequentialService}
	}
}

// CreateServiceWithConfig 根据配置创建验证服务
func (f *ServiceFactory) CreateServiceWithConfig(config *ValidationConfig) Service {
	if config == nil {
		return f.CreateService(SequentialStrategy)
	}

	switch config.Strategy {
	case ConcurrentStrategy:
		concurrentService := concurrent.NewService(f.questionnaireService, config.MaxConcurrency)
		return &ConcurrentAdapter{service: concurrentService}
	case SequentialStrategy:
		fallthrough
	default:
		sequentialService := sequential.NewService(f.questionnaireService)
		return &SequentialAdapter{service: sequentialService}
	}
}

// CreateSequentialService 创建串行验证服务
func (f *ServiceFactory) CreateSequentialService() Service {
	sequentialService := sequential.NewService(f.questionnaireService)
	return &SequentialAdapter{service: sequentialService}
}

// CreateConcurrentService 创建并发验证服务
func (f *ServiceFactory) CreateConcurrentService(maxConcurrency int) Service {
	concurrentService := concurrent.NewService(f.questionnaireService, maxConcurrency)
	return &ConcurrentAdapter{service: concurrentService}
}

// SequentialAdapter 串行验证服务适配器
type SequentialAdapter struct {
	service sequential.Service
}

// ValidateAnswersheet 验证答卷
func (a *SequentialAdapter) ValidateAnswersheet(ctx context.Context, req *ValidationRequest) error {
	// 转换为子包的类型
	sequentialReq := &sequential.ValidationRequest{
		QuestionnaireCode: req.QuestionnaireCode,
		Title:             req.Title,
		TesteeInfo: &sequential.TesteeInfo{
			Name:  req.TesteeInfo.Name,
			Email: req.TesteeInfo.Email,
			Phone: req.TesteeInfo.Phone,
		},
		Answers: make([]*sequential.AnswerValidationItem, len(req.Answers)),
	}

	for i, answer := range req.Answers {
		sequentialReq.Answers[i] = &sequential.AnswerValidationItem{
			QuestionCode: answer.QuestionCode,
			QuestionType: answer.QuestionType,
			Value:        answer.Value,
		}
	}

	return a.service.ValidateAnswersheet(ctx, sequentialReq)
}

// ValidateQuestionnaireCode 验证问卷代码
func (a *SequentialAdapter) ValidateQuestionnaireCode(ctx context.Context, code string) error {
	return a.service.ValidateQuestionnaireCode(ctx, code)
}

// ConcurrentAdapter 并发验证服务适配器
type ConcurrentAdapter struct {
	service concurrent.Service
}

// ValidateAnswersheet 验证答卷
func (a *ConcurrentAdapter) ValidateAnswersheet(ctx context.Context, req *ValidationRequest) error {
	// 转换为子包的类型
	concurrentReq := &concurrent.ValidationRequest{
		QuestionnaireCode: req.QuestionnaireCode,
		Title:             req.Title,
		TesteeInfo: &concurrent.TesteeInfo{
			Name:  req.TesteeInfo.Name,
			Email: req.TesteeInfo.Email,
			Phone: req.TesteeInfo.Phone,
		},
		Answers: make([]*concurrent.AnswerValidationItem, len(req.Answers)),
	}

	for i, answer := range req.Answers {
		concurrentReq.Answers[i] = &concurrent.AnswerValidationItem{
			QuestionCode: answer.QuestionCode,
			QuestionType: answer.QuestionType,
			Value:        answer.Value,
		}
	}

	return a.service.ValidateAnswersheet(ctx, concurrentReq)
}

// ValidateQuestionnaireCode 验证问卷代码
func (a *ConcurrentAdapter) ValidateQuestionnaireCode(ctx context.Context, code string) error {
	return a.service.ValidateQuestionnaireCode(ctx, code)
}
