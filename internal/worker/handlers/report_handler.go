package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	pb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/internalapi"
)

func init() {
	Register("report_generated_handler", func(deps *Dependencies) HandlerFunc {
		return handleReportGenerated(deps)
	})
	Register("report_exported_handler", func(deps *Dependencies) HandlerFunc {
		return handleReportExported(deps)
	})
}

// ==================== Payload 定义 ====================

// ReportGeneratedPayload 报告生成事件数据
type ReportGeneratedPayload struct {
	ReportID     string    `json:"report_id"`
	AssessmentID string    `json:"assessment_id"`
	TesteeID     uint64    `json:"testee_id"`
	ScaleCode    string    `json:"scale_code"`
	ScaleVersion string    `json:"scale_version"`
	TotalScore   float64   `json:"total_score"`
	RiskLevel    string    `json:"risk_level"`
	GeneratedAt  time.Time `json:"generated_at"`
}

// IsHighRisk 是否高风险
func (p ReportGeneratedPayload) IsHighRisk() bool {
	return p.RiskLevel == "high" || p.RiskLevel == "critical"
}

// ReportExportedPayload 报告导出事件数据
type ReportExportedPayload struct {
	ReportID   string    `json:"report_id"`
	ExportType string    `json:"export_type"` // pdf, docx, html
	ExportedBy uint64    `json:"exported_by"`
	ExportedAt time.Time `json:"exported_at"`
}

// ==================== Handler 实现 ====================

func handleReportGenerated(deps *Dependencies) HandlerFunc {
	return func(ctx context.Context, eventType string, payload []byte) error {
		var data ReportGeneratedPayload
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

		// TODO: 发送报告生成通知

		return nil
	}
}

// logReportGenerated 记录报告生成日志
func logReportGenerated(deps *Dependencies, env *EventEnvelope, data ReportGeneratedPayload) {
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
func handleHighRiskAlert(deps *Dependencies, data ReportGeneratedPayload) {
	if !data.IsHighRisk() {
		return
	}

	deps.Logger.Warn("HIGH RISK REPORT GENERATED",
		slog.String("report_id", data.ReportID),
		slog.Uint64("testee_id", data.TesteeID),
		slog.String("risk_level", data.RiskLevel),
		slog.Float64("total_score", data.TotalScore),
	)
	// TODO: 发送预警通知
}

// extractHighRiskFactors 从报告中提取高风险因子
func extractHighRiskFactors(ctx context.Context, deps *Dependencies, data ReportGeneratedPayload) []string {
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

// tagTesteeWithReportData 根据报告数据给受试者打标签
func tagTesteeWithReportData(ctx context.Context, deps *Dependencies, data ReportGeneratedPayload, highRiskFactors []string) {
	markKeyFocus := data.IsHighRisk()

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

func handleReportExported(deps *Dependencies) HandlerFunc {
	return func(ctx context.Context, eventType string, payload []byte) error {
		var data ReportExportedPayload
		env, err := ParseEventData(payload, &data)
		if err != nil {
			return fmt.Errorf("failed to parse report exported event: %w", err)
		}

		deps.Logger.Info("processing report exported",
			slog.String("event_id", env.ID),
			slog.String("report_id", data.ReportID),
			slog.String("export_type", data.ExportType),
			slog.Uint64("exported_by", data.ExportedBy),
			slog.Time("exported_at", data.ExportedAt),
		)

		// TODO: 记录审计日志

		return nil
	}
}
