package grpcclient

import (
	"context"
	"fmt"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/answersheet"
)

// AnswerSheetClient 答卷服务客户端
type AnswerSheetClient struct {
	manager *Manager
	client  pb.AnswerSheetServiceClient
}

// NewAnswerSheetClient 创建答卷服务客户端
func NewAnswerSheetClient(manager *Manager) *AnswerSheetClient {
	return &AnswerSheetClient{
		manager: manager,
		client:  pb.NewAnswerSheetServiceClient(manager.Conn()),
	}
}

// GetAnswerSheet 获取当前填写人拥有的答卷详情。
func (c *AnswerSheetClient) GetAnswerSheet(ctx context.Context, writerID, id uint64) (*pb.GetAnswerSheetResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, c.manager.Timeout())
	defer cancel()

	resp, err := c.client.GetAnswerSheet(ctx, &pb.GetAnswerSheetRequest{
		Id:       id,
		WriterId: writerID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get answer sheet: %w", err)
	}

	return resp, nil
}

// ListAnswerSheets 获取答卷列表
func (c *AnswerSheetClient) ListAnswerSheets(ctx context.Context, req *pb.ListAnswerSheetsRequest) (*pb.ListAnswerSheetsResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, c.manager.Timeout())
	defer cancel()

	resp, err := c.client.ListAnswerSheets(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to list answer sheets: %w", err)
	}

	return resp, nil
}
