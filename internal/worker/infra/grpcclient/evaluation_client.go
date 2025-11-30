package grpcclient

import (
	"context"
	"fmt"

	pb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/evaluation"
)

// EvaluationClient 测评服务客户端
type EvaluationClient struct {
	manager *Manager
	client  pb.EvaluationServiceClient
}

// NewEvaluationClient 创建测评服务客户端
func NewEvaluationClient(manager *Manager) *EvaluationClient {
	return &EvaluationClient{
		manager: manager,
		client:  pb.NewEvaluationServiceClient(manager.Conn()),
	}
}

// GetAssessmentScores 获取测评得分详情
func (c *EvaluationClient) GetAssessmentScores(ctx context.Context, assessmentID uint64) (*pb.GetAssessmentScoresResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, c.manager.Timeout())
	defer cancel()

	resp, err := c.client.GetAssessmentScores(ctx, &pb.GetAssessmentScoresRequest{
		AssessmentId: assessmentID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get assessment scores: %w", err)
	}

	return resp, nil
}

// GetAssessmentReport 获取测评报告
func (c *EvaluationClient) GetAssessmentReport(ctx context.Context, assessmentID uint64) (*pb.GetAssessmentReportResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, c.manager.Timeout())
	defer cancel()

	resp, err := c.client.GetAssessmentReport(ctx, &pb.GetAssessmentReportRequest{
		AssessmentId: assessmentID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get assessment report: %w", err)
	}

	return resp, nil
}
