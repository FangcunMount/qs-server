package handlers

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/pkg/attentionprojection"
)

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
