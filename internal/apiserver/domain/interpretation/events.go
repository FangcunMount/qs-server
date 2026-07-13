package interpretation

import "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"

const (
	EventTypeReportGenerated = eventcatalog.InterpretationReportGenerated
	EventTypeReportFailed    = eventcatalog.InterpretationReportFailed
)

// AggregateType is the durable aggregate root for terminal interpretation
// facts. Artifacts are immutable children of one ReportGeneration.
const AggregateType = "ReportGeneration"
