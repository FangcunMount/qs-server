package decision_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/decision"
)

func TestMatchOutcomeCodeIgnoresPresentationFields(t *testing.T) {
	t.Parallel()
	base := []decision.ScoreRangeRule{
		{MinScore: 0, MaxScore: 40, OutcomeCode: "low"},
		{MinScore: 40, MaxScore: 100, MaxInclusive: true, OutcomeCode: "high"},
	}
	withCopy := []decision.ScoreRangeRule{
		{MinScore: 0, MaxScore: 40, OutcomeCode: "low"},
		{MinScore: 40, MaxScore: 100, MaxInclusive: true, OutcomeCode: "high"},
	}
	// Presentation lives outside ScoreRangeRule; mutating sibling presentation
	// must not change matched OutcomeCode (MC-R016 characterization).
	codeA, okA := decision.MatchOutcomeCode(55, base)
	codeB, okB := decision.MatchOutcomeCode(55, withCopy)
	if !okA || !okB || codeA != "high" || codeB != "high" {
		t.Fatalf("codes = (%q,%v)/(%q,%v), want high", codeA, okA, codeB, okB)
	}
}

func TestMatchOutcomeCodeUsesLevelWhenOutcomeCodeEmpty(t *testing.T) {
	t.Parallel()
	code, ok := decision.MatchOutcomeCode(10, []decision.ScoreRangeRule{
		{MinScore: 0, MaxScore: 100, MaxInclusive: true, Level: "medium"},
	})
	if !ok || code != "medium" {
		t.Fatalf("code = (%q,%v), want medium", code, ok)
	}
}
