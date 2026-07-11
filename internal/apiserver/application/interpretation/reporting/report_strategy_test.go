package reporting

import (
	"testing"

	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationcompat"
)

func TestResolveReportTypeReturnsStandard(t *testing.T) {
	if got := OutcomeReportType(evaloutcome.Outcome{}); got != domainReport.ReportTypeStandard {
		t.Fatalf("resolveReportType() = %s, want %s", got, domainReport.ReportTypeStandard)
	}
}
