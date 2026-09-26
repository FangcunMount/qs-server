package container

import (
	"context"
	"fmt"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/internalapi"
)

type attentionRPC interface {
	SyncAssessmentAttention(context.Context, *pb.SyncAssessmentAttentionRequest) (*pb.SyncAssessmentAttentionResponse, error)
}

type internalAttentionSyncClient struct {
	client attentionRPC
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
	response, err := c.client.SyncAssessmentAttention(ctx, &pb.SyncAssessmentAttentionRequest{
		TesteeId:     testeeID,
		RiskLevel:    riskLevel,
		MarkKeyFocus: markKeyFocus,
	})
	if err != nil {
		return err
	}
	if response == nil || !response.GetSuccess() {
		return fmt.Errorf("attention sync was not accepted by the API")
	}
	return nil
}
