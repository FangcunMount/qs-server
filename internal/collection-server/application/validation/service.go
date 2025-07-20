package validation

import (
	"context"
	"fmt"

	"github.com/yshujie/questionnaire-scale/pkg/log"
)

// Service 校验服务接口
type Service interface {
	// ValidateAnswersheet 校验答卷
	ValidateAnswersheet(ctx context.Context, answersheet *AnswersheetValidationRequest) error
	// ValidateQuestionnaireCode 校验问卷代码
	ValidateQuestionnaireCode(ctx context.Context, code string) error
}

// service 校验服务实现 - 作为协调器
type service struct {
	questionnaireValidator *QuestionnaireValidator
	answerValidator        *AnswerValidator
}

// NewService 创建新的校验服务
func NewService(
	questionnaireValidator *QuestionnaireValidator,
	answerValidator *AnswerValidator,
) Service {
	return &service{
		questionnaireValidator: questionnaireValidator,
		answerValidator:        answerValidator,
	}
}

// AnswersheetValidationRequest 答卷校验请求
type AnswersheetValidationRequest struct {
	QuestionnaireCode string                 `json:"questionnaire_code"`
	Answers           []AnswerValidationItem `json:"answers"`
	TesteeInfo        TesteeInfo             `json:"testee_info"`
}

// AnswerValidationItem 答案校验项
type AnswerValidationItem struct {
	QuestionID   string      `json:"question_id"`
	QuestionType string      `json:"question_type"`
	Value        interface{} `json:"value"`
}

// TesteeInfo 测试者信息
type TesteeInfo struct {
	Name   string `json:"name"`
	Age    int    `json:"age"`
	Gender string `json:"gender"`
	Email  string `json:"email"`
	Phone  string `json:"phone"`
}

// ValidateAnswersheet 校验答卷
func (s *service) ValidateAnswersheet(ctx context.Context, req *AnswersheetValidationRequest) error {
	log.L(ctx).Infof("Validating answersheet for questionnaire: %s", req.QuestionnaireCode)

	// 1. 获取问卷详情（包含问卷代码验证）
	questionnaire, err := s.questionnaireValidator.GetQuestionnaire(ctx, req.QuestionnaireCode)
	if err != nil {
		return fmt.Errorf("questionnaire validation failed: %w", err)
	}

	// 2. 验证答案
	if err := s.answerValidator.ValidateAnswers(ctx, req.Answers, questionnaire); err != nil {
		return fmt.Errorf("answer validation failed: %w", err)
	}

	log.L(ctx).Info("Answersheet validation passed")
	return nil
}

// ValidateQuestionnaireCode 校验问卷代码
func (s *service) ValidateQuestionnaireCode(ctx context.Context, code string) error {
	return s.questionnaireValidator.ValidateQuestionnaireCode(ctx, code)
}
