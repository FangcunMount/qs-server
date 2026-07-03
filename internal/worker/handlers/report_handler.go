package handlers

import (
	"context"
	"fmt"
	"log/slog"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/internalapi"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventoutcome"
	"github.com/FangcunMount/qs-server/internal/pkg/eventpayload"
)

func handleReportGenerated(deps *Dependencies) HandlerFunc {
	return func(ctx context.Context, eventType string, payload []byte) error {
		switch eventType {
		case eventcatalog.ReportGeneratedV2:
			return handleReportGeneratedV2(ctx, deps, payload)
		default:
			return handleReportGeneratedV1(ctx, deps, payload)
		}
	}
}

func handleReportGeneratedV1(ctx context.Context, deps *Dependencies, payload []byte) error {
	var data eventpayload.ReportGeneratedData
	env, err := ParseEventData(payload, &data)
	if err != nil {
		return fmt.Errorf("failed to parse report generated event: %w", err)
	}
	logReportGenerated(deps, env, data)
	handleHighRiskAlert(deps, data.RiskLevel, data.TotalScore, data.ReportID, data.TesteeID)
	if deps.InternalClient != nil {
		syncAssessmentAttention(ctx, deps, data.TesteeID, data.RiskLevel, isHighRiskRiskLevel(data.RiskLevel))
	}
	return nil
}

func handleReportGeneratedV2(ctx context.Context, deps *Dependencies, payload []byte) error {
	var data eventoutcome.ReportGeneratedPayload
	env, err := ParseEventData(payload, &data)
	if err != nil {
		return fmt.Errorf("failed to parse report generated v2 event: %w", err)
	}
	deps.Logger.Info("processing report generated v2",
		slog.String("event_id", env.ID),
		slog.String("report_id", data.ReportID),
		slog.String("assessment_id", data.AssessmentID),
		slog.Uint64("testee_id", data.TesteeID),
		slog.String("level_code", levelCode(data.Level)),
		slog.String("severity", levelSeverity(data.Level)),
	)
	riskLevel := attentionRiskLevelFromV2(data.Level)
	handleHighRiskAlert(deps, riskLevel, primaryScoreValue(data.PrimaryScore), data.ReportID, data.TesteeID)
	if deps.InternalClient != nil {
		syncAssessmentAttention(ctx, deps, data.TesteeID, riskLevel, isHighRiskV2Level(data.Level))
	}
	return nil
}

func logReportGenerated(deps *Dependencies, env *EventEnvelope, data eventpayload.ReportGeneratedData) {
	deps.Logger.Info("processing report generated",
		slog.String("event_id", env.ID),
		slog.String("report_id", data.ReportID),
		slog.String("assessment_id", data.AssessmentID),
		slog.Uint64("testee_id", data.TesteeID),
		slog.Float64("total_score", data.TotalScore),
		slog.String("risk_level", data.RiskLevel),
	)
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
