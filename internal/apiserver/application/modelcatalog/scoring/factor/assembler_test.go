package factor

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/shared"
)

func TestToFactorSnapshotDefaultsAndMapsDTOFields(t *testing.T) {
	maxScore := 10.0

	factor, err := toFactorSnapshot(
		"F1",
		"Factor 1",
		"",
		false,
		true,
		[]string{"Q1", "Q2"},
		"",
		nil,
		&maxScore,
		[]shared.InterpretRuleDTO{{MinScore: 0, MaxScore: 5, RiskLevel: "low", Conclusion: "low", Suggestion: "watch"}},
	)
	if err != nil {
		t.Fatalf("toFactorSnapshot() error = %v", err)
	}

	if factor.Code != "F1" {
		t.Fatalf("factor code = %q, want F1", factor.Code)
	}
	if factor.ScoringStrategy != "sum" {
		t.Fatalf("scoring strategy = %q, want sum", factor.ScoringStrategy)
	}
	if got := len(factor.QuestionCodes); got != 2 {
		t.Fatalf("question code count = %d, want 2", got)
	}
	if got := len(factor.InterpretRules); got != 1 {
		t.Fatalf("interpret rule count = %d, want 1", got)
	}
	if factor.MaxScore == nil || *factor.MaxScore != maxScore {
		t.Fatalf("max score = %#v, want %v", factor.MaxScore, maxScore)
	}
}

func TestToFactorSnapshotRejectsCntStrategyWithoutCntOptionContents(t *testing.T) {
	_, err := toFactorSnapshot(
		"F1",
		"Factor 1",
		"",
		false,
		true,
		[]string{"Q1"},
		"cnt",
		nil,
		nil,
		nil,
	)
	if err == nil {
		t.Fatal("toFactorSnapshot() error = nil, want cnt parameter validation error")
	}
}

func TestToFactorSnapshotAcceptsHistoricalFactorType(t *testing.T) {
	_, err := toFactorSnapshot(
		"F1",
		"Factor 1",
		"first_grade",
		false,
		true,
		[]string{"Q1"},
		"sum",
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("toFactorSnapshot() error = %v", err)
	}
}
