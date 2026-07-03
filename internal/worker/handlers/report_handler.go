package handlers

import (
	"context"
	"fmt"
	"log/slog"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/internalapi"
	"github.com/FangcunMount/qs-server/internal/pkg/eventoutcome"
)

func handleReportGenerated(deps *Dependencies) HandlerFunc {
	return func(ctx context.Context, _ string, payload []byte) error {
		return handleReportGeneratedOutcome(ctx, deps, payload)
	}
}

func handleReportGeneratedOutcome(ctx context.Context, deps *Dependencies, payload []byte) error {
	var data eventoutcome.ReportGeneratedPayload
	env, err := ParseEventData(payload, &data)
	if err != nil {
		return fmt.Errorf("failed to parse report generated outcome event: %w", err)
	}
	deps.Logger.Info("processing report generated outcome",
		slog.String("event_id", env.ID),
		slog.String("report_id", data.ReportID),
		slog.String("assessment_id", data.AssessmentID),
		slog.Uint64("testee_id", data.TesteeID),
		slog.String("level_code", levelCode(data.Level)),
		slog.String("severity", levelSeverity(data.Level)),
	)
	riskLevel := attentionRiskLevelFromOutcome(data.Level)
	handleHighRiskAlert(deps, riskLevel, primaryScoreValue(data.PrimaryScore), data.ReportID, data.TesteeID)
	if deps.InternalClient != nil {
		syncAssessmentAttention(ctx, deps, data.TesteeID, riskLevel, isHighRiskOutcomeLevel(data.Level))
	}
	return nil
}

func handleHighRiskAlert(deps *Dependencies, riskLevel string, totalScore float64, reportID string, testeeID uint64) {
	if !isHighRiskRiskLevel(riskLevel) {
		return
	}
	deps.Logger.Warn("HIGH RISK REPORT GENERATED",
		slog.String("report_id", reportID),
		slog.Uint64("testee_id", testeeID),
		slog.String("risk_level", riskLevel),
		slog.Float64("total_score", totalScore),
	)
}

func syncAssessmentAttention(ctx context.Context, deps *Dependencies, testeeID uint64, riskLevel string, markKeyFocus bool) {
	resp, err := deps.InternalClient.SyncAssessmentAttention(
		ctx,
		&pb.SyncAssessmentAttentionRequest{
			TesteeId:     testeeID,
			RiskLevel:    riskLevel,
			MarkKeyFocus: markKeyFocus,
		},
	)
	if err != nil {
		deps.Logger.Warn("failed to sync assessment attention",
			slog.Uint64("testee_id", testeeID),
			slog.String("error", err.Error()),
		)
		return
	}
	deps.Logger.Info("assessment attention synced successfully",
		slog.Uint64("testee_id", testeeID),
		slog.Bool("key_focus_marked", resp.KeyFocusMarked),
	)
}

func primaryScoreValue(score *eventoutcome.ScoreValue) float64 {
	if score == nil {
		return 0
	}
	return score.Value
}

func levelCode(level *eventoutcome.ResultLevel) string {
	if level == nil {
		return ""
	}
	return level.Code
}

func levelSeverity(level *eventoutcome.ResultLevel) string {
	if level == nil {
		return ""
	}
	return level.Severity
}
