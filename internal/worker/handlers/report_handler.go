package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/report"
	pb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/internalapi"
)

func handleReportGenerated(deps *Dependencies) HandlerFunc {
	return func(ctx context.Context, _ string, payload []byte) error {
		var data domainReport.ReportGeneratedData
		env, err := ParseEventData(payload, &data)
		if err != nil {
			return fmt.Errorf("failed to parse report generated event: %w", err)
		}

		// 记录报告生成日志
		logReportGenerated(deps, env, data)

		// 处理高风险预警
		handleHighRiskAlert(deps, data)

		// 同步测评后置关注状态；高风险队列不再依赖受试者 tag。
		if deps.InternalClient != nil {
			syncAssessmentAttentionWithReportData(ctx, deps, data)
		}

		return nil
	}
}

// logReportGenerated 记录报告生成日志
func logReportGenerated(deps *Dependencies, env *EventEnvelope, data domainReport.ReportGeneratedData) {
	deps.Logger.Info("processing report generated",
		slog.String("event_id", env.ID),
		slog.String("report_id", data.ReportID),
		slog.String("assessment_id", data.AssessmentID),
		slog.Uint64("testee_id", data.TesteeID),
		slog.Float64("total_score", data.TotalScore),
		slog.String("risk_level", data.RiskLevel),
	)
}

// handleHighRiskAlert 处理高风险预警
func handleHighRiskAlert(deps *Dependencies, data domainReport.ReportGeneratedData) {
	if !isHighRiskRiskLevel(data.RiskLevel) {
		return
	}

	deps.Logger.Warn("HIGH RISK REPORT GENERATED",
		slog.String("report_id", data.ReportID),
		slog.Uint64("testee_id", data.TesteeID),
		slog.String("risk_level", data.RiskLevel),
		slog.Float64("total_score", data.TotalScore),
	)
}

func isHighRiskRiskLevel(riskLevel string) bool {
	switch strings.ToLower(strings.TrimSpace(riskLevel)) {
	case "high", "severe":
		return true
	default:
		return false
	}
}

// syncAssessmentAttentionWithReportData 根据报告数据同步测评后置关注状态。
func syncAssessmentAttentionWithReportData(ctx context.Context, deps *Dependencies, data domainReport.ReportGeneratedData) {
	markKeyFocus := isHighRiskRiskLevel(data.RiskLevel)

	resp, err := deps.InternalClient.SyncAssessmentAttention(
		ctx,
		&pb.SyncAssessmentAttentionRequest{
			TesteeId:     data.TesteeID,
			RiskLevel:    data.RiskLevel,
			MarkKeyFocus: markKeyFocus,
		},
	)
	if err != nil {
		deps.Logger.Warn("failed to sync assessment attention",
			slog.String("report_id", data.ReportID),
			slog.Uint64("testee_id", data.TesteeID),
			slog.String("error", err.Error()),
		)
		return
	}

	deps.Logger.Info("assessment attention synced successfully",
		slog.String("report_id", data.ReportID),
		slog.Uint64("testee_id", data.TesteeID),
		slog.Bool("key_focus_marked", resp.KeyFocusMarked),
	)
}
