package sequential

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/questionnaire"
)

// Service 串行验证服务接口
type Service interface {
	// ValidateAnswersheet 验证答卷
	ValidateAnswersheet(ctx context.Context, req *ValidationRequest) error

	// ValidateQuestionnaireCode 验证问卷代码
	ValidateQuestionnaireCode(ctx context.Context, code string) error
}

// service 串行验证服务实现
type service struct {
	validator *Validator
}

// NewService 创建串行验证服务
func NewService(questionnaireService questionnaire.Service) Service {
	return &service{
		validator: NewValidator(questionnaireService),
	}
}

// ValidateAnswersheet 验证答卷
func (s *service) ValidateAnswersheet(ctx context.Context, req *ValidationRequest) error {
	return s.validator.ValidateAnswersheet(ctx, req)
}

// ValidateQuestionnaireCode 验证问卷代码
func (s *service) ValidateQuestionnaireCode(ctx context.Context, code string) error {
	return s.validator.ValidateQuestionnaireCode(ctx, code)
}
