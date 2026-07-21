package handlers

import (
	"context"
	"fmt"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/internalapi"
	"github.com/FangcunMount/qs-server/internal/pkg/attentionprojection"
)

type internalAttentionSyncClient struct {
	client InternalClient
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

func projectAssessmentAttention(
	ctx context.Context,
	deps *Dependencies,
	eventID string,
	data attentionReportGeneratedData,
) error {
	if deps == nil || deps.InternalClient == nil {
		return nil
	}
	if deps.AttentionProjector != nil {
		return deps.AttentionProjector.Project(ctx, attentionprojection.PendingInput{
			EventID:      eventID,
			ReportID:     data.ReportID,
			AssessmentID: data.AssessmentID,
			TesteeID:     data.TesteeID,
			RiskLevel:    data.RiskLevel,
			MarkKeyFocus: data.MarkKeyFocus,
		})
	}
	syncAssessmentAttention(ctx, deps, data.TesteeID, data.RiskLevel, data.MarkKeyFocus)
	return nil
}

type attentionReportGeneratedData struct {
	ReportID     string
	AssessmentID string
	TesteeID     uint64
	RiskLevel    string
	MarkKeyFocus bool
}
