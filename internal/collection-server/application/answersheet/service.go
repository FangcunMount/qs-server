package answersheet

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/FangcunMount/iam-contracts/pkg/log"
	answersheetpb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/answersheet"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/questionnaire"
	"github.com/FangcunMount/qs-server/internal/collection-server/domain/answersheet"
	"github.com/FangcunMount/qs-server/internal/collection-server/infrastructure/grpc"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	internalpubsub "github.com/FangcunMount/qs-server/internal/pkg/pubsub"
	"github.com/FangcunMount/qs-server/pkg/pubsub"
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
	answersheetClient    grpc.AnswersheetClient
	questionnaireService questionnaire.Service
	validator            *answersheet.Validator
	publisher            pubsub.Publisher
}

// NewService 创建答卷应用服务
func NewService(answersheetClient grpc.AnswersheetClient, publisher pubsub.Publisher, questionnaireService questionnaire.Service) Service {
	return &service{
		answersheetClient:    answersheetClient,
		questionnaireService: questionnaireService,
		validator:            answersheet.NewValidator(),
		publisher:            publisher,
	}
}

// SubmitAnswersheet 提交答卷
func (s *service) SubmitAnswersheet(ctx context.Context, req *SubmitRequest) (*SubmitResponse, error) {
	log.L(ctx).Info("=== Starting answersheet submission process ===")

	if req == nil {
		log.L(ctx).Error("Submit request is nil")
		return nil, fmt.Errorf("submit request cannot be nil")
	}

	log.L(ctx).Infof("Validating submit request for questionnaire: %s", req.QuestionnaireCode)
	// 验证请求
	if err := s.validateSubmitRequest(req); err != nil {
		log.L(ctx).Errorf("Request validation failed: %v", err)
		return nil, fmt.Errorf("invalid submit request: %w", err)
	}
	log.L(ctx).Info("Request validation passed")

	log.L(ctx).Infof("Getting questionnaire info for validation: %s", req.QuestionnaireCode)
	// 获取问卷信息用于验证
	questionnaireInfo, err := s.questionnaireService.GetQuestionnaireForValidation(ctx, req.QuestionnaireCode)
	if err != nil {
		log.L(ctx).Errorf("Failed to get questionnaire info: %v", err)
		return nil, fmt.Errorf("failed to get questionnaire for validation: %w", err)
	}
	log.L(ctx).Infof("Successfully retrieved questionnaire info: %s", req.QuestionnaireCode)

	log.L(ctx).Info("Converting to domain entity...")
	// 转换为领域实体
	answersheetEntity := s.convertToAnswersheet(req)
	log.L(ctx).Infof("Domain entity created with %d answers", len(answersheetEntity.Answers))

	log.L(ctx).Info("Starting domain validation...")
	// 验证答卷
	if err := s.validator.ValidateSubmitRequest(ctx, answersheetEntity, questionnaireInfo); err != nil {
		log.L(ctx).Errorf("Domain validation failed: %v", err)
		return nil, fmt.Errorf("answersheet validation failed: %w", err)
	}
	log.L(ctx).Info("Domain validation passed")

	log.L(ctx).Info("Converting to gRPC request...")
	// 转换为gRPC请求
	grpcReq, err := s.convertToSaveRequest(req)
	if err != nil {
		log.L(ctx).Errorf("Failed to convert to gRPC request: %v", err)
		return nil, fmt.Errorf("failed to convert request: %w", err)
	}
	log.L(ctx).Infof("gRPC request prepared: questionnaire_code=%s, answers_count=%d",
		grpcReq.QuestionnaireCode, len(grpcReq.Answers))

	// 调用gRPC客户端保存答卷
	log.L(ctx).Infof("Calling gRPC SaveAnswersheet: questionnaire_code=%s", req.QuestionnaireCode)
	grpcResp, err := s.answersheetClient.SaveAnswersheet(ctx, grpcReq)
	if err != nil {
		log.L(ctx).Errorf("gRPC SaveAnswersheet failed: %v", err)
		return nil, fmt.Errorf("failed to save answersheet: %w", err)
	}

	log.L(ctx).Infof("Successfully saved answersheet via gRPC: id=%d, message=%s",
		grpcResp.Id, grpcResp.Message)

	// 发布答卷已保存消息
	if s.publisher != nil {
		log.L(ctx).Info("Publishing answersheet saved message...")
		if err := s.publishAnswersheetSavedMessage(ctx, req, grpcResp.Id); err != nil {
			log.L(ctx).Errorf("Failed to publish answersheet saved message: %v", err)
			// 不影响主流程，只记录错误
		} else {
			log.L(ctx).Info("Answersheet saved message published successfully")
		}
	}

	// 转换响应
	response := &SubmitResponse{
		ID:        meta.ID(grpcResp.Id),
		Status:    "success",
		Message:   grpcResp.Message,
		CreatedAt: time.Now(),
	}

	log.L(ctx).Infof("=== Answersheet submission completed successfully: id=%s ===", response.ID)
	return response, nil
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

	// 获取问卷信息
	questionnaireInfo, err := s.questionnaireService.GetQuestionnaireForValidation(ctx, req.QuestionnaireCode)
	if err != nil {
		return fmt.Errorf("failed to get questionnaire for validation: %w", err)
	}

	// 验证答卷
	if err := s.validator.ValidateSubmitRequest(ctx, answersheetEntity, questionnaireInfo); err != nil {
		return fmt.Errorf("answersheet validation failed: %w", err)
	}

	return nil
}

// convertToSaveRequest 将DTO转换为gRPC保存请求
func (s *service) convertToSaveRequest(req *SubmitRequest) (*answersheetpb.SaveAnswerSheetRequest, error) {
	// 转换答案列表
	grpcAnswers := make([]*answersheetpb.Answer, len(req.Answers))
	for i, answer := range req.Answers {
		// 将答案值转换为JSON字符串
		valueJSON, err := json.Marshal(answer.Value)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal answer value: %w", err)
		}

		grpcAnswers[i] = &answersheetpb.Answer{
			QuestionCode: answer.QuestionCode,
			QuestionType: answer.QuestionType,
			Score:        0, // 分数在后续计算中设置
			Value:        string(valueJSON),
		}
	}

	// TODO: 这里需要获取实际的用户ID，暂时使用默认值
	writerID := uint64(1) // 答卷填写者ID
	testeeID := uint64(1) // 被测试者ID

	return &answersheetpb.SaveAnswerSheetRequest{
		QuestionnaireCode:    req.QuestionnaireCode,
		QuestionnaireVersion: "1.0", // 暂时使用默认版本
		Title:                req.Title,
		WriterId:             writerID,
		TesteeId:             testeeID,
		Answers:              grpcAnswers,
	}, nil
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

	// 处理Age字段，从指针转换为int
	age := 0
	if req.TesteeInfo.Age != nil {
		age = *req.TesteeInfo.Age
	}

	return &answersheet.SubmitRequest{
		QuestionnaireCode: req.QuestionnaireCode,
		Title:             req.Title,
		TesteeInfo: &answersheet.TesteeInfo{
			Name:   req.TesteeInfo.Name,
			Gender: req.TesteeInfo.Gender,
			Age:    age,
			Email:  req.TesteeInfo.Email,
			Phone:  req.TesteeInfo.Phone,
		},
		Answers: answers,
	}
}

// publishAnswersheetSavedMessage 发布答卷已保存消息
func (s *service) publishAnswersheetSavedMessage(ctx context.Context, req *SubmitRequest, answersheetID uint64) error {
	// 创建答卷已保存数据
	answersheetData := &internalpubsub.AnswersheetSavedData{
		ResponseID:           meta.ID(answersheetID).String(),
		QuestionnaireCode:    req.QuestionnaireCode,
		QuestionnaireVersion: "1.0",
		AnswerSheetID:        answersheetID,
		WriterID:             1, // TODO: 从上下文获取实际用户ID
		SubmittedAt:          time.Now().Unix(),
	}

	// 创建答卷已保存消息
	message := internalpubsub.NewAnswersheetSavedMessage(
		internalpubsub.SourceCollectionServer,
		answersheetData,
	)

	// 发布消息
	if err := s.publisher.Publish(ctx, "answersheet.saved", message); err != nil {
		return fmt.Errorf("failed to publish message: %w", err)
	}

	log.L(ctx).Infof("Published answersheet saved message for response ID: %s", answersheetData.ResponseID)
	return nil
}
