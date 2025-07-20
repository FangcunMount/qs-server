package validation

import (
	"context"
	"fmt"

	"github.com/yshujie/questionnaire-scale/pkg/log"
)

// ServiceConcurrent 并发校验服务接口
type ServiceConcurrent interface {
	// ValidateAnswersheet 并发校验答卷
	ValidateAnswersheet(ctx context.Context, answersheet *AnswersheetValidationRequest) error
	// ValidateQuestionnaireCode 校验问卷代码
	ValidateQuestionnaireCode(ctx context.Context, code string) error
	// GetMaxConcurrency 获取最大并发数
	GetMaxConcurrency() int
	// SetMaxConcurrency 设置最大并发数
	SetMaxConcurrency(maxConcurrency int)
}

// serviceConcurrent 并发校验服务实现
type serviceConcurrent struct {
	questionnaireValidator *QuestionnaireValidator
	answerValidator        *AnswerValidatorConcurrent
}

// NewServiceConcurrent 创建新的并发校验服务
func NewServiceConcurrent(
	questionnaireValidator *QuestionnaireValidator,
	answerValidator *AnswerValidatorConcurrent,
) ServiceConcurrent {
	return &serviceConcurrent{
		questionnaireValidator: questionnaireValidator,
		answerValidator:        answerValidator,
	}
}

// ValidateAnswersheet 并发校验答卷
func (s *serviceConcurrent) ValidateAnswersheet(ctx context.Context, req *AnswersheetValidationRequest) error {
	log.L(ctx).Infof("Starting concurrent validation for questionnaire: %s", req.QuestionnaireCode)

	// 1. 获取问卷详情（包含问卷代码验证）
	questionnaire, err := s.questionnaireValidator.GetQuestionnaire(ctx, req.QuestionnaireCode)
	if err != nil {
		return fmt.Errorf("questionnaire validation failed: %w", err)
	}

	// 2. 并发验证答案
	if err := s.answerValidator.ValidateAnswers(ctx, req.Answers, questionnaire); err != nil {
		return fmt.Errorf("answer validation failed: %w", err)
	}

	log.L(ctx).Info("Concurrent answersheet validation passed")
	return nil
}

// ValidateQuestionnaireCode 校验问卷代码
func (s *serviceConcurrent) ValidateQuestionnaireCode(ctx context.Context, code string) error {
	return s.questionnaireValidator.ValidateQuestionnaireCode(ctx, code)
}

// GetMaxConcurrency 获取最大并发数
func (s *serviceConcurrent) GetMaxConcurrency() int {
	return s.answerValidator.GetMaxConcurrency()
}

// SetMaxConcurrency 设置最大并发数
func (s *serviceConcurrent) SetMaxConcurrency(maxConcurrency int) {
	s.answerValidator.SetMaxConcurrency(maxConcurrency)
}
