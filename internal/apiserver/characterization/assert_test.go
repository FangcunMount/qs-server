package characterization_test

import (
	"testing"

	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
)

func assertSuggestionExists(
	t *testing.T,
	suggestions []domainreport.Suggestion,
	category domainreport.SuggestionCategory,
	content string,
) {
	t.Helper()
	for _, s := range suggestions {
		if s.Category == category && s.Content == content {
			return
		}
	}
	t.Fatalf("missing suggestion category=%s content=%q in %#v", category, content, suggestions)
}

func assertDimensionField(
	t *testing.T,
	dim domainreport.DimensionInterpret,
	wantName string,
	wantScore float64,
	wantRisk domainreport.RiskLevel,
	wantDescription string,
) {
	t.Helper()
	if dim.FactorName() != wantName {
		t.Fatalf("FactorName = %q, want %q", dim.FactorName(), wantName)
	}
	if dim.RawScore() != wantScore {
		t.Fatalf("RawScore = %v, want %v", dim.RawScore(), wantScore)
	}
	if dim.RiskLevel() != wantRisk {
		t.Fatalf("RiskLevel = %s, want %s", dim.RiskLevel(), wantRisk)
	}
	if dim.Description() != wantDescription {
		t.Fatalf("Description = %q, want %q", dim.Description(), wantDescription)
	}
}
