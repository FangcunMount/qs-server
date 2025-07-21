package sequential

import (
	"context"
	"fmt"

	"github.com/yshujie/questionnaire-scale/internal/collection-server/application/questionnaire"
	"github.com/yshujie/questionnaire-scale/internal/collection-server/application/validation"
	"github.com/yshujie/questionnaire-scale/internal/collection-server/domain/answersheet"
	"github.com/yshujie/questionnaire-scale/pkg/log"
)

// Service 串行验证服务
type Service struct {
	questionnaireService questionnaire.Service
	answersheetValidator *answersheet.Validator
}

// NewService 创建串行验证服务
func NewService(questionnaireService questionnaire.Service) validation.Service {
	return &Service{
		questionnaireService: questionnaireService,
		answersheetValidator: answersheet.NewValidator(),
	}
}

// ValidateAnswersheet 验证答卷
func (s *Service) ValidateAnswersheet(ctx context.Context, req *validation.ValidationRequest) error {
	if req == nil {
		return fmt.Errorf("validation request cannot be nil")
	}

	log.L(ctx).Infof("Validating answersheet for questionnaire: %s (sequential)", req.QuestionnaireCode)

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

	// 4. 使用领域验证器进行验证
	if err := s.answersheetValidator.ValidateSubmitRequest(ctx, answersheetEntity, questionnaireInfo); err != nil {
		return fmt.Errorf("answersheet validation failed: %w", err)
	}

	log.L(ctx).Info("Sequential validation completed successfully")
	return nil
}

// ValidateQuestionnaireCode 验证问卷代码
func (s *Service) ValidateQuestionnaireCode(ctx context.Context, code string) error {
	return s.questionnaireService.ValidateQuestionnaireCode(ctx, code)
}

// convertToAnswersheet 将请求转换为答卷实体
func (s *Service) convertToAnswersheet(req *validation.ValidationRequest) *answersheet.SubmitRequest {
	answers := make([]*answersheet.Answer, 0, len(req.Answers))
	for _, answer := range req.Answers {
		answers = append(answers, &answersheet.Answer{
			QuestionCode: answer.QuestionCode,
			QuestionType: answer.QuestionType,
			Value:        answer.Value,
		})
	}

	return &answersheet.SubmitRequest{
		QuestionnaireCode: req.QuestionnaireCode,
		Title:             req.Title,
		TesteeInfo: &answersheet.TesteeInfo{
			Name:  req.TesteeInfo.Name,
			Email: req.TesteeInfo.Email,
			Phone: req.TesteeInfo.Phone,
		},
		Answers: answers,
	}
}
