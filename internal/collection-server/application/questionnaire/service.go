package questionnaire

import (
	"context"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/collection-server/domain/answersheet"
	"github.com/FangcunMount/qs-server/internal/collection-server/domain/questionnaire"
	"github.com/FangcunMount/qs-server/internal/collection-server/infrastructure/grpc"
)

// Service 问卷应用服务接口
type Service interface {
	// GetQuestionnaire 获取问卷信息
	GetQuestionnaire(ctx context.Context, code string) (*questionnaire.Questionnaire, error)

	// ValidateQuestionnaireCode 验证问卷代码
	ValidateQuestionnaireCode(ctx context.Context, code string) error

	// GetQuestionnaireForValidation 获取用于验证的问卷信息
	GetQuestionnaireForValidation(ctx context.Context, code string) (answersheet.QuestionnaireInfo, error)
}

// service 问卷应用服务实现
type service struct {
	questionnaireClient grpc.QuestionnaireClient
	validator           *questionnaire.Validator
}

// NewService 创建问卷应用服务
func NewService(questionnaireClient grpc.QuestionnaireClient) Service {
	return &service{
		questionnaireClient: questionnaireClient,
		validator:           questionnaire.NewValidator(),
	}
}

// GetQuestionnaire 获取问卷信息
func (s *service) GetQuestionnaire(ctx context.Context, code string) (*questionnaire.Questionnaire, error) {
	if code == "" {
		return nil, fmt.Errorf("questionnaire code cannot be empty")
	}

	// 通过 gRPC 获取问卷信息
	resp, err := s.questionnaireClient.GetQuestionnaire(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("failed to get questionnaire from apiserver: %w", err)
	}

	if resp.Questionnaire == nil {
		return nil, fmt.Errorf("questionnaire not found: %s", code)
	}

	// 转换为领域实体
	questionnaireEntity := questionnaire.FromProto(resp.Questionnaire)
	if questionnaireEntity == nil {
		return nil, fmt.Errorf("failed to convert questionnaire from proto")
	}

	// 验证问卷实体
	if err := s.validator.ValidateQuestionnaire(questionnaireEntity); err != nil {
		return nil, fmt.Errorf("invalid questionnaire: %w", err)
	}

	return questionnaireEntity, nil
}

// ValidateQuestionnaireCode 验证问卷代码
func (s *service) ValidateQuestionnaireCode(ctx context.Context, code string) error {
	if code == "" {
		return fmt.Errorf("questionnaire code cannot be empty")
	}

	// 验证问卷代码格式
	if err := s.validator.ValidateCode(code); err != nil {
		return fmt.Errorf("invalid questionnaire code format: %w", err)
	}

	// 检查问卷是否存在
	_, err := s.GetQuestionnaire(ctx, code)
	if err != nil {
		return fmt.Errorf("questionnaire not found or invalid: %w", err)
	}

	return nil
}

// GetQuestionnaireForValidation 获取用于验证的问卷信息
func (s *service) GetQuestionnaireForValidation(ctx context.Context, code string) (answersheet.QuestionnaireInfo, error) {
	questionnaireEntity, err := s.GetQuestionnaire(ctx, code)
	if err != nil {
		return nil, err
	}

	// 返回适配器，实现 QuestionnaireInfo 接口
	return questionnaire.NewQuestionnaireAdapter(questionnaireEntity), nil
}
