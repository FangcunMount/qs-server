package grpcclient

import (
	"context"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/internalapi"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/planentry"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type PlanEntryClient struct {
	client     *Client
	grpcClient pb.PlanEntryServiceClient
}

func NewPlanEntryClient(client *Client) *PlanEntryClient {
	return &PlanEntryClient{client: client, grpcClient: pb.NewPlanEntryServiceClient(client.Conn())}
}

func (c *PlanEntryClient) ResolveTaskEntry(ctx context.Context, taskID, token string) (*planentry.Entry, error) {
	ctx, cancel := c.client.ContextWithTimeout(ctx)
	defer cancel()
	result, err := c.grpcClient.ResolveTaskEntry(ctx, &pb.ResolveTaskEntryRequest{TaskId: taskID, Token: token})
	if status.Code(err) == codes.NotFound {
		return nil, planentry.ErrInvalidEntry
	}
	if err != nil {
		return nil, err
	}
	return &planentry.Entry{
		TaskID: result.GetTaskId(), TesteeID: result.GetTesteeId(),
		ScaleCode: result.GetScaleCode(), ExpiresAt: result.GetExpiresAt(),
	}, nil
}
