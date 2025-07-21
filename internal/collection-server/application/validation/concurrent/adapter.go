package concurrent

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/collection-server/application/questionnaire"
)

// ValidatorAdapter 并发验证器适配器
// 将 Validator 适配为 Service 接口
type ValidatorAdapter struct {
	validator *Validator
}

// NewValidatorAdapter 创建并发验证器适配器
func NewValidatorAdapter(questionnaireService questionnaire.Service, maxConcurrency int) Service {
	validator := NewValidator(questionnaireService, maxConcurrency)
	return &ValidatorAdapter{
		validator: validator,
	}
}

// ValidateAnswersheet 验证答卷
func (a *ValidatorAdapter) ValidateAnswersheet(ctx context.Context, req *ValidationRequest) error {
	return a.validator.ValidateAnswersheet(ctx, req)
}

// ValidateQuestionnaireCode 验证问卷代码
func (a *ValidatorAdapter) ValidateQuestionnaireCode(ctx context.Context, code string) error {
	return a.validator.ValidateQuestionnaireCode(ctx, code)
}

// GetValidator 获取底层验证器（提供更多功能）
func (a *ValidatorAdapter) GetValidator() *Validator {
	return a.validator
}

// GetMaxConcurrency 获取最大并发数
func (a *ValidatorAdapter) GetMaxConcurrency() int {
	return a.validator.GetMaxConcurrency()
}

// SetMaxConcurrency 设置最大并发数
func (a *ValidatorAdapter) SetMaxConcurrency(maxConcurrency int) {
	a.validator.SetMaxConcurrency(maxConcurrency)
}

// ServiceAdapter 并发服务适配器
// 将并发验证器适配为通用的验证服务接口
type ServiceAdapter struct {
	validator *Validator
}

// NewServiceAdapter 创建并发服务适配器
func NewServiceAdapter(questionnaireService questionnaire.Service, maxConcurrency int) *ServiceAdapter {
	validator := NewValidator(questionnaireService, maxConcurrency)
	return &ServiceAdapter{
		validator: validator,
	}
}

// ValidateAnswersheet 验证答卷
func (a *ServiceAdapter) ValidateAnswersheet(ctx context.Context, req interface{}) error {
	// 类型转换
	concurrentReq, ok := req.(*ValidationRequest)
	if !ok {
		// 这里可能需要从通用类型转换
		return a.validator.ValidateAnswersheet(ctx, concurrentReq)
	}
	return a.validator.ValidateAnswersheet(ctx, concurrentReq)
}

// ValidateQuestionnaireCode 验证问卷代码
func (a *ServiceAdapter) ValidateQuestionnaireCode(ctx context.Context, code string) error {
	return a.validator.ValidateQuestionnaireCode(ctx, code)
}

// GetConcurrencyInfo 获取并发信息
func (a *ServiceAdapter) GetConcurrencyInfo() map[string]interface{} {
	return map[string]interface{}{
		"strategy":        "concurrent",
		"max_concurrency": a.validator.GetMaxConcurrency(),
		"validator_type":  "concurrent",
	}
}
