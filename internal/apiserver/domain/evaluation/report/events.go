package report

import (
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/eventconfig"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// ==================== 事件类型常量 ====================
// 从 eventconfig 包导入，保持事件类型的单一来源

const (
	// EventTypeGenerated 报告已生成
	EventTypeGenerated = eventconfig.ReportGenerated
	// EventTypeExported 报告已导出
	EventTypeExported = eventconfig.ReportExported
)

// AggregateType 聚合根类型
const AggregateType = "Report"

// ==================== 事件 Payload 定义 ====================

// ReportGeneratedData 报告已生成事件数据
type ReportGeneratedData struct {
	ReportID     string    `json:"report_id"`
	AssessmentID string    `json:"assessment_id"`
	TesteeID     uint64    `json:"testee_id"`
	ScaleCode    string    `json:"scale_code"`
	ScaleVersion string    `json:"scale_version"`
	TotalScore   float64   `json:"total_score"`
	RiskLevel    string    `json:"risk_level"`
	GeneratedAt  time.Time `json:"generated_at"`
}

// ReportExportedData 报告已导出事件数据
type ReportExportedData struct {
	ReportID   string    `json:"report_id"`
	ExportType string    `json:"export_type"` // pdf, docx, html
	ExportedBy uint64    `json:"exported_by"` // 导出人ID
	ExportedAt time.Time `json:"exported_at"`
}

// ==================== 事件类型别名 ====================

// ReportGeneratedEvent 报告已生成事件
type ReportGeneratedEvent = event.Event[ReportGeneratedData]

// ReportExportedEvent 报告已导出事件
type ReportExportedEvent = event.Event[ReportExportedData]

// ==================== 事件构造函数 ====================

// NewReportGeneratedEvent 创建报告已生成事件
func NewReportGeneratedEvent(
	reportID string,
	assessmentID string,
	testeeID uint64,
	scaleCode string,
	scaleVersion string,
	totalScore float64,
	riskLevel string,
	generatedAt time.Time,
) ReportGeneratedEvent {
	return event.New(EventTypeGenerated, AggregateType, reportID,
		ReportGeneratedData{
			ReportID:     reportID,
			AssessmentID: assessmentID,
			TesteeID:     testeeID,
			ScaleCode:    scaleCode,
			ScaleVersion: scaleVersion,
			TotalScore:   totalScore,
			RiskLevel:    riskLevel,
			GeneratedAt:  generatedAt,
		},
	)
}

// NewReportExportedEvent 创建报告已导出事件
func NewReportExportedEvent(
	reportID string,
	exportType string,
	exportedBy uint64,
	exportedAt time.Time,
) ReportExportedEvent {
	return event.New(EventTypeExported, AggregateType, reportID,
		ReportExportedData{
			ReportID:   reportID,
			ExportType: exportType,
			ExportedBy: exportedBy,
			ExportedAt: exportedAt,
		},
	)
}
