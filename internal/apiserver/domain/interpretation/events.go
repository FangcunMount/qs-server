package interpretation

import "github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"

const (
	EventTypeReportGenerated = eventcatalog.InterpretationReportGenerated
	EventTypeReportFailed    = eventcatalog.InterpretationReportFailed
)

// AggregateType 聚合根类型
const AggregateType = "Report"
