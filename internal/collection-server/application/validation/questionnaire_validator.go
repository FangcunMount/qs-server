package validation

import (
	"context"
	"fmt"

	questionnairepb "github.com/yshujie/questionnaire-scale/internal/apiserver/interface/grpc/proto/questionnaire"
)

// QuestionnaireClient 问卷客户端接口（重用已有接口）
type QuestionnaireClient interface {
	GetQuestionnaire(ctx context.Context, code string) (*questionnairepb.GetQuestionnaireResponse, error)
}

// QuestionnaireValidator 问卷验证器
type QuestionnaireValidator struct {
	client QuestionnaireClient
}

// NewQuestionnaireValidator 创建问卷验证器
func NewQuestionnaireValidator(client QuestionnaireClient) *QuestionnaireValidator {
	return &QuestionnaireValidator{
		client: client,
	}
}

// ValidateQuestionnaireCode 验证问卷代码
func (v *QuestionnaireValidator) ValidateQuestionnaireCode(ctx context.Context, code string) error {
	if code == "" {
		return fmt.Errorf("questionnaire code cannot be empty")
	}

	// 校验代码长度
	if len(code) < 3 || len(code) > 50 {
		return fmt.Errorf("questionnaire code length must be between 3 and 50 characters")
	}

	return nil
}

// GetQuestionnaire 获取问卷
func (v *QuestionnaireValidator) GetQuestionnaire(ctx context.Context, code string) (*questionnairepb.Questionnaire, error) {
	if err := v.ValidateQuestionnaireCode(ctx, code); err != nil {
		return nil, fmt.Errorf("invalid questionnaire code: %w", err)
	}

	response, err := v.client.GetQuestionnaire(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("failed to get questionnaire: %w", err)
	}

	if response.Questionnaire == nil {
		return nil, fmt.Errorf("questionnaire not found: %s", code)
	}

	return response.Questionnaire, nil
}
