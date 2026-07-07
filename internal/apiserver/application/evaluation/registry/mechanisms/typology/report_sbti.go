// characterization-only: typology report builder helper for V1 contract tests.
package typology

import (
	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
)

// NewSBTIReportBuilder is a characterization helper for typology reports.
func NewSBTIReportBuilder() interpretationreporting.ReportBuilder {
	builder, err := NewConfiguredReportBuilder()
	if err != nil {
		panic(err)
	}
	return builder
}
