package sequential

import (
	"context"
	"fmt"

	"github.com/yshujie/questionnaire-scale/internal/collection-server/application/questionnaire"
	"github.com/yshujie/questionnaire-scale/internal/collection-server/domain/answersheet"
	"github.com/yshujie/questionnaire-scale/pkg/log"
)

// ValidationRequest 验证请求
type ValidationRequest struct {
	QuestionnaireCode string                  `json:"questionnaire_code" validate:"required"`
	Title             string                  `json:"title" validate:"required"`
	TesteeInfo        *TesteeInfo             `json:"testee_info" validate:"required"`
	Answers           []*AnswerValidationItem `json:"answers" validate:"required"`
}

// TesteeInfo 测试者信息
type TesteeInfo struct {
	Name  string `json:"name" validate:"required"`
	Email string `json:"email,omitempty"`
	Phone string `json:"phone,omitempty"`
}

// AnswerValidationItem 答案验证项
type AnswerValidationItem struct {
	QuestionCode string      `json:"question_code" validate:"required"`
	QuestionType string      `json:"question_type" validate:"required"`
	Value        interface{} `json:"value" validate:"required"`
}

// Validator 串行验证器
type Validator struct {
	questionnaireService questionnaire.Service
	answersheetValidator *answersheet.Validator
}

// NewValidator 创建串行验证器
func NewValidator(questionnaireService questionnaire.Service) *Validator {
	return &Validator{
		questionnaireService: questionnaireService,
		answersheetValidator: answersheet.NewValidator(),
	}
}

// ValidateAnswersheet 验证答卷
func (v *Validator) ValidateAnswersheet(ctx context.Context, req *ValidationRequest) error {
	if req == nil {
		return fmt.Errorf("validation request cannot be nil")
	}

	log.L(ctx).Infof("Validating answersheet for questionnaire: %s (sequential)", req.QuestionnaireCode)

	// 1. 验证问卷代码
	if err := v.ValidateQuestionnaireCode(ctx, req.QuestionnaireCode); err != nil {
		return fmt.Errorf("questionnaire code validation failed: %w", err)
	}

	// 2. 获取问卷信息用于验证
	questionnaireInfo, err := v.questionnaireService.GetQuestionnaireForValidation(ctx, req.QuestionnaireCode)
	if err != nil {
		return fmt.Errorf("failed to get questionnaire for validation: %w", err)
	}

	// 3. 转换为领域实体
	answersheetEntity := v.convertToAnswersheet(req)

	// 4. 使用领域验证器进行串行验证
	if err := v.answersheetValidator.ValidateSubmitRequest(ctx, answersheetEntity, questionnaireInfo); err != nil {
		return fmt.Errorf("answersheet validation failed: %w", err)
	}

	log.L(ctx).Info("Sequential validation completed successfully")
	return nil
}

// ValidateQuestionnaireCode 验证问卷代码
func (v *Validator) ValidateQuestionnaireCode(ctx context.Context, code string) error {
	return v.questionnaireService.ValidateQuestionnaireCode(ctx, code)
}

// convertToAnswersheet 将请求转换为答卷实体
func (v *Validator) convertToAnswersheet(req *ValidationRequest) *answersheet.SubmitRequest {
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
