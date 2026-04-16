package grpcclient

import (
	"context"
	"fmt"
	"time"

	pb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/internalapi"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// InternalClient 内部服务客户端
// 用于 Worker 调用 APIServer 的内部接口
type InternalClient struct {
	manager *Manager
	client  pb.InternalServiceClient
}

// NewInternalClient 创建内部服务客户端
func NewInternalClient(manager *Manager) *InternalClient {
	return &InternalClient{
		manager: manager,
		client:  pb.NewInternalServiceClient(manager.Conn()),
	}
}

// CreateAssessmentFromAnswerSheet 从答卷创建测评
func (c *InternalClient) CreateAssessmentFromAnswerSheet(
	ctx context.Context,
	req *pb.CreateAssessmentFromAnswerSheetRequest,
) (*pb.CreateAssessmentFromAnswerSheetResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, c.manager.Timeout())
	defer cancel()

	resp, err := c.client.CreateAssessmentFromAnswerSheet(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create assessment from answersheet: %w", err)
	}

	return resp, nil
}

// EvaluateAssessment 执行测评评估
func (c *InternalClient) EvaluateAssessment(
	ctx context.Context,
	assessmentID uint64,
) (*pb.EvaluateAssessmentResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, c.manager.Timeout())
	defer cancel()

	resp, err := c.client.EvaluateAssessment(ctx, &pb.EvaluateAssessmentRequest{
		AssessmentId: assessmentID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate assessment: %w", err)
	}

	return resp, nil
}

// CalculateAnswerSheetScore 计算答卷分数
func (c *InternalClient) CalculateAnswerSheetScore(
	ctx context.Context,
	req *pb.CalculateAnswerSheetScoreRequest,
) (*pb.CalculateAnswerSheetScoreResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, c.manager.Timeout())
	defer cancel()

	resp, err := c.client.CalculateAnswerSheetScore(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate answersheet score: %w", err)
	}

	return resp, nil
}

// TagTestee 给受试者打标签
func (c *InternalClient) TagTestee(
	ctx context.Context,
	req *pb.TagTesteeRequest,
) (*pb.TagTesteeResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, c.manager.Timeout())
	defer cancel()

	resp, err := c.client.TagTestee(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to tag testee: %w", err)
	}

	return resp, nil
}

// GenerateQuestionnaireQRCode 生成问卷小程序码
func (c *InternalClient) GenerateQuestionnaireQRCode(
	ctx context.Context,
	code, version string,
) (*pb.GenerateQuestionnaireQRCodeResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, c.manager.Timeout())
	defer cancel()

	resp, err := c.client.GenerateQuestionnaireQRCode(ctx, &pb.GenerateQuestionnaireQRCodeRequest{
		Code:    code,
		Version: version,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to generate questionnaire QR code: %w", err)
	}

	return resp, nil
}

// GenerateScaleQRCode 生成量表小程序码
func (c *InternalClient) GenerateScaleQRCode(
	ctx context.Context,
	code string,
) (*pb.GenerateScaleQRCodeResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, c.manager.Timeout())
	defer cancel()

	resp, err := c.client.GenerateScaleQRCode(ctx, &pb.GenerateScaleQRCodeRequest{
		Code: code,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to generate scale QR code: %w", err)
	}

	return resp, nil
}

func (c *InternalClient) ProjectBehaviorEvent(
	ctx context.Context,
	req *pb.ProjectBehaviorEventRequest,
) (*pb.ProjectBehaviorEventResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, c.manager.Timeout())
	defer cancel()

	resp, err := c.client.ProjectBehaviorEvent(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to project behavior event: %w", err)
	}

	return resp, nil
}

// SendTaskOpenedMiniProgramNotification 发送 task.opened 小程序订阅消息。
func (c *InternalClient) SendTaskOpenedMiniProgramNotification(
	ctx context.Context,
	orgID int64,
	taskID string,
	testeeID uint64,
	entryURL string,
	openAt time.Time,
) (*pb.SendTaskOpenedMiniProgramNotificationResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, c.manager.Timeout())
	defer cancel()

	resp, err := c.client.SendTaskOpenedMiniProgramNotification(ctx, &pb.SendTaskOpenedMiniProgramNotificationRequest{
		OrgId:    orgID,
		TaskId:   taskID,
		TesteeId: testeeID,
		EntryUrl: entryURL,
		OpenAt:   timestamppb.New(openAt),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to send task opened mini program notification: %w", err)
	}

	return resp, nil
}
