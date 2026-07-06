package interpretation

import "github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"

const (
	// EventTypeGenerated 报告已生成（outcome 投影见 events_outcome.go）
	EventTypeGenerated = eventcatalog.ReportGenerated
)

// AggregateType 聚合根类型
const AggregateType = "Report"
