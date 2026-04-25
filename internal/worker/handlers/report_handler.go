package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

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

		// 提取高风险因子并给受试者打标签
		if deps.InternalClient != nil {
			highRiskFactors := extractHighRiskFactors(ctx, deps, data)
			tagTesteeWithReportData(ctx, deps, data, highRiskFactors)
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

// extractHighRiskFactors 从报告中提取高风险因子
func extractHighRiskFactors(ctx context.Context, deps *Dependencies, data domainReport.ReportGeneratedData) []string {
	if deps.EvaluationClient == nil {
		return nil
	}

	assessmentID, err := strconv.ParseUint(data.AssessmentID, 10, 64)
	if err != nil {
		deps.Logger.Warn("failed to parse assessment_id",
			slog.String("report_id", data.ReportID),
			slog.String("assessment_id", data.AssessmentID),
			slog.String("error", err.Error()),
		)
		return nil
	}

	reportResp, err := deps.EvaluationClient.GetAssessmentReport(ctx, assessmentID)
	if err != nil {
		deps.Logger.Warn("failed to get report for factor extraction",
			slog.String("report_id", data.ReportID),
			slog.String("assessment_id", data.AssessmentID),
			slog.String("error", err.Error()),
		)
		return nil
	}

	if reportResp == nil || reportResp.Report == nil {
		return nil
	}

	var highRiskFactors []string
	for _, dim := range reportResp.Report.Dimensions {
		if isHighRiskDimension(dim.RiskLevel) && dim.FactorCode != "" {
			highRiskFactors = append(highRiskFactors, dim.FactorCode)
		}
	}

	if len(highRiskFactors) > 0 {
		deps.Logger.Info("extracted high risk factors from report",
			slog.String("report_id", data.ReportID),
			slog.Int("factor_count", len(highRiskFactors)),
			slog.Any("factors", highRiskFactors),
		)
	}

	return highRiskFactors
}

// isHighRiskDimension 判断维度风险等级是否为高风险
func isHighRiskDimension(riskLevel string) bool {
	return riskLevel == "high" || riskLevel == "severe"
}

func isHighRiskRiskLevel(riskLevel string) bool {
	return riskLevel == "high" || riskLevel == "critical"
}

// tagTesteeWithReportData 根据报告数据给受试者打标签
func tagTesteeWithReportData(ctx context.Context, deps *Dependencies, data domainReport.ReportGeneratedData, highRiskFactors []string) {
	markKeyFocus := isHighRiskRiskLevel(data.RiskLevel)

	resp, err := deps.InternalClient.TagTestee(
		ctx,
		&pb.TagTesteeRequest{
			TesteeId:        data.TesteeID,
			RiskLevel:       data.RiskLevel,
			ScaleCode:       data.ScaleCode,
			MarkKeyFocus:    markKeyFocus,
			HighRiskFactors: highRiskFactors,
		},
	)
	if err != nil {
		deps.Logger.Warn("failed to tag testee",
			slog.String("report_id", data.ReportID),
			slog.Uint64("testee_id", data.TesteeID),
			slog.String("error", err.Error()),
		)
		return
	}

	deps.Logger.Info("testee tagged successfully",
		slog.String("report_id", data.ReportID),
		slog.Uint64("testee_id", data.TesteeID),
		slog.Any("tags_added", resp.TagsAdded),
		slog.Bool("key_focus_marked", resp.KeyFocusMarked),
	)
}
