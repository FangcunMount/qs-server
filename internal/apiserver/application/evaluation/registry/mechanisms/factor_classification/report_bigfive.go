// characterization-only: typology report builder helper for V1 contract tests.
package factor_classification

import (
	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
)

// NewBigFiveReportBuilder is a characterization helper for typology reports.
func NewBigFiveReportBuilder() interpretationreporting.ReportBuilder {
	builder, err := NewConfiguredReportBuilder()
	if err != nil {
		panic(err)
	}
	return builder
}
