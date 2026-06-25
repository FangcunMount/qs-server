package report

import (
	"testing"

	rulesetmbti "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset/mbti"
)

func TestBuildMBTIReportFillsModelExtra(t *testing.T) {
	detail := MBTIReportDetail{
		TypeCode:     "INTJ",
		TypeName:     "建筑师",
		OneLiner:     "独立战略家",
		MatchPercent: 75,
		Profile: rulesetmbti.TypeProfileSnapshot{
			TypeCode: "INTJ",
			TypeName: "建筑师",
			Summary:  "善于长远规划",
		},
		Source: rulesetmbti.SourceSnapshot{
			Attribution:   "OEJTS",
			License:       "CC BY-NC-SA 4.0",
			NonCommercial: true,
		},
	}

	report, err := BuildMBTIReport(MBTIReportInput{
		AssessmentID: ID(7001),
		ModelCode:    "MBTI_OEJTS",
		Detail:       detail,
	})
	if err != nil {
		t.Fatalf("BuildMBTIReport: %v", err)
	}
	extra := report.ModelExtra()
	if extra == nil {
		t.Fatal("expected model extra")
	}
	if extra.Kind != "mbti" || extra.TypeCode != "INTJ" || extra.TypeName != "建筑师" {
		t.Fatalf("unexpected model extra: %#v", extra)
	}
	if extra.MatchPercent != 75 {
		t.Fatalf("MatchPercent = %v, want 75", extra.MatchPercent)
	}
}
