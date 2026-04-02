package grpcclient

import (
	"context"
	"fmt"

	pb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/internalapi"
)

// PlanClient plan 写侧命令客户端。
type PlanClient struct {
	manager *Manager
	client  pb.PlanCommandServiceClient
}

func NewPlanClient(manager *Manager) *PlanClient {
	return &PlanClient{
		manager: manager,
		client:  pb.NewPlanCommandServiceClient(manager.Conn()),
	}
}

func (c *PlanClient) SchedulePendingTasks(
	ctx context.Context,
	req *pb.SchedulePendingTasksRequest,
) (*pb.SchedulePendingTasksResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, c.manager.Timeout())
	defer cancel()

	resp, err := c.client.SchedulePendingTasks(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to schedule pending plan tasks: %w", err)
	}
	return resp, nil
}
