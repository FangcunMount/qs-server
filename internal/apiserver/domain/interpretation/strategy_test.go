package interpretation

import "testing"

func TestResolveReportTypeReturnsStandard(t *testing.T) {
	if got := ResolveReportType(); got != ReportTypeStandard {
		t.Fatalf("ResolveReportType() = %s, want %s", got, ReportTypeStandard)
	}
}
