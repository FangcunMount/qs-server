package report

import (
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// ==================== 事件类型常量 ====================
// 从 eventcatalog 包导入，保持事件类型的单一来源

const (
	// EventTypeGenerated 报告已生成
	EventTypeGenerated = eventcatalog.ReportGenerated
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

// ==================== 事件类型别名 ====================

// ReportGeneratedEvent 报告已生成事件
type ReportGeneratedEvent = event.Event[ReportGeneratedData]

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
