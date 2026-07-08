package reporting

import (
	"testing"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
)

func TestResolveReportTypeReturnsStandard(t *testing.T) {
	if got := OutcomeReportType(evaloutcome.Outcome{}); got != domainReport.ReportTypeStandard {
		t.Fatalf("resolveReportType() = %s, want %s", got, domainReport.ReportTypeStandard)
	}
}
