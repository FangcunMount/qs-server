package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"
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

		deps.Logger.Info("processing report generated",
			slog.String("event_id", env.ID),
			slog.String("report_id", data.ReportID),
			slog.String("assessment_id", data.AssessmentID),
			slog.Uint64("testee_id", data.TesteeID),
			slog.Float64("total_score", data.TotalScore),
			slog.String("risk_level", data.RiskLevel),
		)

		// 高风险预警
		if data.IsHighRisk() {
			deps.Logger.Warn("HIGH RISK REPORT GENERATED",
				slog.String("report_id", data.ReportID),
				slog.Uint64("testee_id", data.TesteeID),
				slog.String("risk_level", data.RiskLevel),
				slog.Float64("total_score", data.TotalScore),
			)
			// TODO: 发送预警通知
		}

		// 给受试者打标签
		if deps.InternalClient != nil {
			markKeyFocus := data.IsHighRisk() // 高风险时自动标记为重点关注

			// 查询报告详情，提取高风险因子
			var highRiskFactors []string
			if deps.EvaluationClient != nil {
				// 解析 assessmentID（字符串格式）
				assessmentID, err := strconv.ParseUint(data.AssessmentID, 10, 64)
				if err == nil {
					reportResp, err := deps.EvaluationClient.GetAssessmentReport(ctx, assessmentID)
					if err == nil && reportResp != nil && reportResp.Report != nil {
						// 提取高风险因子编码
						for _, dim := range reportResp.Report.Dimensions {
							// 判断是否为高风险（high 或 severe）
							riskLevel := dim.RiskLevel
							if riskLevel == "high" || riskLevel == "severe" {
								if dim.FactorCode != "" {
									highRiskFactors = append(highRiskFactors, dim.FactorCode)
								}
							}
						}
						deps.Logger.Info("extracted high risk factors from report",
							slog.String("report_id", data.ReportID),
							slog.Int("factor_count", len(highRiskFactors)),
							slog.Any("factors", highRiskFactors),
						)
					} else if err != nil {
						deps.Logger.Warn("failed to get report for factor extraction",
							slog.String("report_id", data.ReportID),
							slog.String("assessment_id", data.AssessmentID),
							slog.String("error", err.Error()),
						)
					}
				} else {
					deps.Logger.Warn("failed to parse assessment_id",
						slog.String("report_id", data.ReportID),
						slog.String("assessment_id", data.AssessmentID),
						slog.String("error", err.Error()),
					)
				}
			}

			resp, err := deps.InternalClient.TagTestee(
				ctx,
				data.TesteeID,
				data.RiskLevel,
				data.ScaleCode,
				markKeyFocus,
				highRiskFactors,
			)
			if err != nil {
				deps.Logger.Warn("failed to tag testee",
					slog.String("report_id", data.ReportID),
					slog.Uint64("testee_id", data.TesteeID),
					slog.String("error", err.Error()),
				)
				// 不影响主流程，继续执行
			} else {
				deps.Logger.Info("testee tagged successfully",
					slog.String("report_id", data.ReportID),
					slog.Uint64("testee_id", data.TesteeID),
					slog.Any("tags_added", resp.TagsAdded),
					slog.Bool("key_focus_marked", resp.KeyFocusMarked),
				)
			}
		}

		// TODO: 发送报告生成通知

		return nil
	}
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
