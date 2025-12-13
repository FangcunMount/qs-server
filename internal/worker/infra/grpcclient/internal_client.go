package grpcclient

import (
	"context"
	"fmt"

	pb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/internalapi"
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
