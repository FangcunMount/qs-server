package scale

import (
	"testing"

	domainScale "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
)

func TestToFactorDomainDefaultsAndMapsDTOFields(t *testing.T) {
	maxScore := 10.0

	factor, err := toFactorDomain(
		"F1",
		"Factor 1",
		"",
		false,
		true,
		[]string{"Q1", "Q2"},
		"",
		nil,
		&maxScore,
		[]InterpretRuleDTO{{MinScore: 0, MaxScore: 5, RiskLevel: "low", Conclusion: "low", Suggestion: "watch"}},
	)
	if err != nil {
		t.Fatalf("toFactorDomain() error = %v", err)
	}

	if factor.GetCode().String() != "F1" || factor.GetFactorType() != domainScale.FactorTypePrimary {
		t.Fatalf("unexpected factor identity/type: %#v %q", factor.GetCode(), factor.GetFactorType())
	}
	if factor.GetScoringStrategy() != domainScale.ScoringStrategySum {
		t.Fatalf("scoring strategy = %q, want sum", factor.GetScoringStrategy())
	}
	if got := len(factor.GetQuestionCodes()); got != 2 {
		t.Fatalf("question code count = %d, want 2", got)
	}
	if got := len(factor.GetInterpretRules()); got != 1 {
		t.Fatalf("interpret rule count = %d, want 1", got)
	}
	if factor.GetMaxScore() == nil || *factor.GetMaxScore() != maxScore {
		t.Fatalf("max score = %#v, want %v", factor.GetMaxScore(), maxScore)
	}
}

func TestToFactorDomainRejectsCntStrategyWithoutCntOptionContents(t *testing.T) {
	_, err := toFactorDomain(
		"F1",
		"Factor 1",
		"",
		false,
		true,
		nil,
		domainScale.ScoringStrategyCnt.String(),
		nil,
		nil,
		nil,
	)
	if err == nil {
		t.Fatal("toFactorDomain() error = nil, want cnt parameter validation error")
	}
}
