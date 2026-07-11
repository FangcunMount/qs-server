package input

import (
	"testing"

	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
)

func TestFactorScoresPreferCanonicalDimensions(t *testing.T) {
	items := factorScores(&domainoutcome.Execution{Dimensions: []domainoutcome.DimensionResult{
		{Code: "gec", Name: "GEC", Role: "index", HierarchyLevel: 1, Score: &domainoutcome.ScoreValue{Value: 10}, Level: &domainoutcome.ResultLevel{Code: "medium"}},
		{Code: "bri", Name: "BRI", Role: "index", ParentCode: "gec", HierarchyLevel: 2, Score: &domainoutcome.ScoreValue{Value: 8}},
	}}, nil)
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
