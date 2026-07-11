package assessment

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
)

func TestReportRowToResultMapsModelExtra(t *testing.T) {
	got := reportRowToResult(evaluationreadmodel.ReportRow{
		AssessmentID: 303,
		ModelCode:    "SBTI_FUN",
		ModelExtra: &evaluationreadmodel.ReportModelExtraRow{
			Kind:           "sbti",
			TypeCode:       "HIGH",
			TypeName:       "高能者",
			IsSpecial:      false,
			SpecialTrigger: "",
			Commentary:     "commentary",
		},
	})
	if got == nil || got.ModelExtra == nil {
		t.Fatal("expected model extra on report result")
	}
	if got.ModelExtra.Kind != "sbti" || got.ModelExtra.TypeCode != "HIGH" || got.ModelExtra.Commentary != "commentary" {
		t.Fatalf("model extra = %#v", got.ModelExtra)
	}
}
