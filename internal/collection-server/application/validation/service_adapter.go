package validation

import (
	"context"
)

// ServiceAdapter 服务适配器，让并发服务实现原有Service接口
type ServiceAdapter struct {
	concurrentService ServiceConcurrent
}

// NewServiceAdapter 创建服务适配器
func NewServiceAdapter(concurrentService ServiceConcurrent) Service {
	return &ServiceAdapter{
		concurrentService: concurrentService,
	}
}

// ValidateAnswersheet 校验答卷（委托给并发服务）
func (a *ServiceAdapter) ValidateAnswersheet(ctx context.Context, answersheet *AnswersheetValidationRequest) error {
	return a.concurrentService.ValidateAnswersheet(ctx, answersheet)
}

// ValidateQuestionnaireCode 校验问卷代码（委托给并发服务）
func (a *ServiceAdapter) ValidateQuestionnaireCode(ctx context.Context, code string) error {
	return a.concurrentService.ValidateQuestionnaireCode(ctx, code)
}
