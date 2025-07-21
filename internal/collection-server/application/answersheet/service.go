package answersheet

import (
	"context"
	"fmt"
	"time"

	"github.com/yshujie/questionnaire-scale/internal/collection-server/domain/answersheet"
	"github.com/yshujie/questionnaire-scale/internal/collection-server/infrastructure/grpc"
)

// Service 答卷应用服务接口
type Service interface {
	// SubmitAnswersheet 提交答卷
	SubmitAnswersheet(ctx context.Context, req *SubmitRequest) (*SubmitResponse, error)

	// ValidateAnswersheet 验证答卷
	ValidateAnswersheet(ctx context.Context, req *ValidationRequest) error
}

// service 答卷应用服务实现
type service struct {
	answersheetClient grpc.AnswersheetClient
	validator         *answersheet.Validator
}

// NewService 创建答卷应用服务
func NewService(answersheetClient grpc.AnswersheetClient) Service {
	return &service{
		answersheetClient: answersheetClient,
		validator:         answersheet.NewValidator(),
	}
}

// SubmitAnswersheet 提交答卷
func (s *service) SubmitAnswersheet(ctx context.Context, req *SubmitRequest) (*SubmitResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("submit request cannot be nil")
	}

	// 验证请求
	if err := s.validateSubmitRequest(req); err != nil {
		return nil, fmt.Errorf("invalid submit request: %w", err)
	}

	// 转换为领域实体
	answersheetEntity := s.convertToAnswersheet(req)

	// 验证答卷（这里需要问卷信息，暂时传 nil）
	if err := s.validator.ValidateSubmitRequest(ctx, answersheetEntity, nil); err != nil {
		return nil, fmt.Errorf("answersheet validation failed: %w", err)
	}

	// TODO: 实现 gRPC 调用逻辑
	// 暂时返回模拟响应
	return &SubmitResponse{
		ID:        "mock-id-123",
		Status:    "success",
		Message:   "Answersheet submitted successfully",
		CreatedAt: time.Now(),
	}, nil
}

// ValidateAnswersheet 验证答卷
func (s *service) ValidateAnswersheet(ctx context.Context, req *ValidationRequest) error {
	if req == nil {
		return fmt.Errorf("validation request cannot be nil")
	}

	// 验证请求
	if err := s.validateValidationRequest(req); err != nil {
		return fmt.Errorf("invalid validation request: %w", err)
	}

	// 转换为领域实体
	answersheetEntity := s.convertToAnswersheet(&SubmitRequest{
		QuestionnaireCode: req.QuestionnaireCode,
		Title:             req.Title,
		TesteeInfo:        req.TesteeInfo,
		Answers:           req.Answers,
	})

	// 验证答卷（这里需要问卷信息，暂时传 nil）
	if err := s.validator.ValidateSubmitRequest(ctx, answersheetEntity, nil); err != nil {
		return fmt.Errorf("answersheet validation failed: %w", err)
	}

	return nil
}

// validateSubmitRequest 验证提交请求
func (s *service) validateSubmitRequest(req *SubmitRequest) error {
	if req.QuestionnaireCode == "" {
		return fmt.Errorf("questionnaire code cannot be empty")
	}

	if req.Title == "" {
		return fmt.Errorf("title cannot be empty")
	}

	if len(req.Title) > 200 {
		return fmt.Errorf("title cannot exceed 200 characters")
	}

	if req.TesteeInfo == nil {
		return fmt.Errorf("testee info cannot be nil")
	}

	if req.TesteeInfo.Name == "" {
		return fmt.Errorf("testee name cannot be empty")
	}

	if len(req.Answers) == 0 {
		return fmt.Errorf("answers cannot be empty")
	}

	return nil
}

// validateValidationRequest 验证验证请求
func (s *service) validateValidationRequest(req *ValidationRequest) error {
	if req.QuestionnaireCode == "" {
		return fmt.Errorf("questionnaire code cannot be empty")
	}

	if req.Title == "" {
		return fmt.Errorf("title cannot be empty")
	}

	if len(req.Title) > 200 {
		return fmt.Errorf("title cannot exceed 200 characters")
	}

	if req.TesteeInfo == nil {
		return fmt.Errorf("testee info cannot be nil")
	}

	if req.TesteeInfo.Name == "" {
		return fmt.Errorf("testee name cannot be empty")
	}

	if len(req.Answers) == 0 {
		return fmt.Errorf("answers cannot be empty")
	}

	return nil
}

// convertToAnswersheet 将请求转换为答卷实体
func (s *service) convertToAnswersheet(req *SubmitRequest) *answersheet.SubmitRequest {
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
