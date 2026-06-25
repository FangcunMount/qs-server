package result

import (
	"testing"

	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
)

func TestResolveReportTypeReturnsStandard(t *testing.T) {
	if got := resolveReportType(Outcome{}); got != domainReport.ReportTypeStandard {
		t.Fatalf("resolveReportType() = %s, want %s", got, domainReport.ReportTypeStandard)
	}
}
