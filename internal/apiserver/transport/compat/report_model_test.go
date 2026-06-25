package compat

import "testing"

func TestReportModelCompatMappingPreservesValues(t *testing.T) {
	const name = "MBTI 人格类型测评 - 建筑师"
	const code = "MBTI_OEJTS"
	if got := ReportScaleName(name); got != name {
		t.Fatalf("ReportScaleName() = %q, want %q", got, name)
	}
	if got := ReportScaleCode(code); got != code {
		t.Fatalf("ReportScaleCode() = %q, want %q", got, code)
	}
}
