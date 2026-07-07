package interpretation

import "github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"

const (
	// EventTypeGenerated 报告已生成（结果 投影见 events_结果.go）
	EventTypeGenerated = eventcatalog.ReportGenerated
)

// AggregateType 聚合根类型
const AggregateType = "Report"
