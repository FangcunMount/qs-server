package input

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
)

func TestPreviewFactorScoresPreferCanonicalDimensions(t *testing.T) {
	execution := &evaluationfact.Execution{Dimensions: []evaluationfact.DimensionResult{
		{Code: "gec", Name: "GEC", Role: "index", HierarchyLevel: 1, Score: &evaluationfact.ScoreValue{Value: 10}, Level: &evaluationfact.ResultLevel{Code: "medium"}},
		{Code: "bri", Name: "BRI", Role: "index", ParentCode: "gec", HierarchyLevel: 2, Score: &evaluationfact.ScoreValue{Value: 8}},
	}}
	items := factorScores(execution, nil)
	if len(items) != 2 {
		t.Fatalf("factor scores = %d, want 2", len(items))
	}
	if items[1].ParentCode != "gec" || items[1].HierarchyLevel != 2 {
		t.Fatalf("child score = %#v, want hierarchy metadata", items[1])
	}
	if items[0].RiskLevel != report.RiskLevelMedium {
		t.Fatalf("risk level = %s, want medium", items[0].RiskLevel)
	}
}
