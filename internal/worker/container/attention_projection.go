package container

import (
	"context"
	"fmt"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/internalapi"
	"github.com/FangcunMount/qs-server/internal/worker/infra/grpcclient"
)

type internalAttentionSyncClient struct {
	client *grpcclient.InternalClient
}

func (c *internalAttentionSyncClient) SyncAssessmentAttention(
	ctx context.Context,
	testeeID uint64,
	riskLevel string,
	markKeyFocus bool,
) error {
	if c == nil || c.client == nil {
		return fmt.Errorf("internal attention client is not configured")
	}
	_, err := c.client.SyncAssessmentAttention(ctx, &pb.SyncAssessmentAttentionRequest{
		TesteeId:     testeeID,
		RiskLevel:    riskLevel,
		MarkKeyFocus: markKeyFocus,
	})
	return err
}
