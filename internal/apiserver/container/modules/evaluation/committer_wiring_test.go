package evaluation

import (
	"os"
	"strings"
	"testing"
)

func TestProductionEvaluationUsesCommitterAndEvaluationOwnedScoreProjection(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("assemble.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, required := range []string{
		"outcomecommit.NewCommitter(",
		"outcomescoring.NewAssessmentScoreProjector(",
		"execute.WithEvaluationCommitter(",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("evaluation production wiring must contain %q", required)
		}
	}
	for _, forbidden := range []string{
		"MaterializeScoreProjectors(",
		"NewScoreProjectorRegistry(",
		"execute.WithScoringWriter(",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("evaluation production wiring still uses interpretation-owned score path %q", forbidden)
		}
	}
}
