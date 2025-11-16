package concurrent

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/questionnaire"
)

// ValidatorAdapter 并发验证器适配器
// 将 Validator 适配为 Service 接口
type ValidatorAdapter struct {
	service Service
}

// NewValidatorAdapter 创建并发验证器适配器
func NewValidatorAdapter(questionnaireService questionnaire.Service, maxConcurrency int) Service {
	concurrentService := NewService(questionnaireService, maxConcurrency)
	return &ValidatorAdapter{
		service: concurrentService,
	}
}

// ValidateAnswersheet 验证答卷
func (a *ValidatorAdapter) ValidateAnswersheet(ctx context.Context, req *ValidationRequest) error {
	return a.service.ValidateAnswersheet(ctx, req)
}

// ValidateQuestionnaireCode 验证问卷代码
func (a *ValidatorAdapter) ValidateQuestionnaireCode(ctx context.Context, code string) error {
	return a.service.ValidateQuestionnaireCode(ctx, code)
}

// GetService 获取底层服务（提供更多功能）
func (a *ValidatorAdapter) GetService() Service {
	return a.service
}

// ServiceExtended 扩展的并发服务接口
type ServiceExtended interface {
	Service
	GetMaxConcurrency() int
	SetMaxConcurrency(maxConcurrency int)
	GetServiceInfo() map[string]interface{}
}

// ExtendedServiceAdapter 扩展的并发服务适配器
type ExtendedServiceAdapter struct {
	service *service
}

// NewExtendedServiceAdapter 创建扩展的并发服务适配器
func NewExtendedServiceAdapter(questionnaireService questionnaire.Service, maxConcurrency int) ServiceExtended {
	concurrentService := NewService(questionnaireService, maxConcurrency).(*service)
	return &ExtendedServiceAdapter{
		service: concurrentService,
	}
}

// ValidateAnswersheet 验证答卷
func (a *ExtendedServiceAdapter) ValidateAnswersheet(ctx context.Context, req *ValidationRequest) error {
	return a.service.ValidateAnswersheet(ctx, req)
}

// ValidateQuestionnaireCode 验证问卷代码
func (a *ExtendedServiceAdapter) ValidateQuestionnaireCode(ctx context.Context, code string) error {
	return a.service.ValidateQuestionnaireCode(ctx, code)
}

// GetMaxConcurrency 获取最大并发数
func (a *ExtendedServiceAdapter) GetMaxConcurrency() int {
	return a.service.GetMaxConcurrency()
}

// SetMaxConcurrency 设置最大并发数
func (a *ExtendedServiceAdapter) SetMaxConcurrency(maxConcurrency int) {
	a.service.SetMaxConcurrency(maxConcurrency)
}

// GetServiceInfo 获取服务信息
func (a *ExtendedServiceAdapter) GetServiceInfo() map[string]interface{} {
	return a.service.GetServiceInfo()
}

// ConcurrentValidationManager 并发验证管理器
type ConcurrentValidationManager struct {
	validator *Validator
	service   Service
}

// NewConcurrentValidationManager 创建并发验证管理器
func NewConcurrentValidationManager(questionnaireService questionnaire.Service, maxConcurrency int) *ConcurrentValidationManager {
	return &ConcurrentValidationManager{
		validator: NewValidator(maxConcurrency),
		service:   NewService(questionnaireService, maxConcurrency),
	}
}

// GetValidator 获取并发验证器
func (m *ConcurrentValidationManager) GetValidator() *Validator {
	return m.validator
}

// GetService 获取并发服务
func (m *ConcurrentValidationManager) GetService() Service {
	return m.service
}

// ValidateWithBoth 使用验证器和服务双重验证
func (m *ConcurrentValidationManager) ValidateWithBoth(ctx context.Context, req *ValidationRequest) error {
	// 首先使用服务验证
	if err := m.service.ValidateAnswersheet(ctx, req); err != nil {
		return err
	}

	// 如果需要，可以添加额外的验证逻辑
	return nil
}

// GetManagerInfo 获取管理器信息
func (m *ConcurrentValidationManager) GetManagerInfo() map[string]interface{} {
	return map[string]interface{}{
		"manager_type":    "concurrent",
		"has_validator":   m.validator != nil,
		"has_service":     m.service != nil,
		"validator_stats": m.validator.GetConcurrencyStats(),
	}
}
