package snapshot

import "testing"

func TestFindFactor(t *testing.T) {
	scale := &ScaleSnapshot{
		Factors: []FactorSnapshot{{Code: "F1"}},
	}
	got, ok := scale.FindFactor("F1")
	if !ok || got.Code != "F1" {
		t.Fatalf("FindFactor = %+v, %v", got, ok)
	}
}

func TestInterpretRuleSnapshotMatchesLeftClosedRightOpen(t *testing.T) {
	rule := InterpretRuleSnapshot{Min: 0, Max: 10}
	if !rule.Matches(0) || !rule.Matches(9.9) || rule.Matches(10) {
		t.Fatal("expected [0,10) semantics: min inclusive, max exclusive")
	}
}

func TestFactorSnapshotCanonicalRoundTrip(t *testing.T) {
	t.Parallel()

	original := FactorSnapshot{
		Code: "f1", ScoringStrategy: "cnt_option",
		ScoringParams:  ScoringParamsSnapshot{CntOptionContents: []string{"yes"}},
		InterpretRules: []InterpretRuleSnapshot{{Min: 0, Max: 1, RiskLevel: "low"}},
	}
	got := FactorSnapshotFromCanonical(original.Canonical())
	if got.Code != original.Code || got.InterpretRules[0].RiskLevel != "low" {
		t.Fatalf("round trip = %#v", got)
	}
}

func TestFactorSnapshotDomainCanonicalRoundTripPreservesExecutionShape(t *testing.T) {
	t.Parallel()

	maxScore := 10.0
	original := FactorSnapshot{
		Code:            "f1",
		Title:           "维度一",
		IsTotalScore:    true,
		QuestionCodes:   []string{"q1", "q2"},
		ScoringStrategy: "cnt",
		ScoringParams: ScoringParamsSnapshot{
			CntOptionContents: []string{"yes", "no"},
		},
		MaxScore: &maxScore,
		InterpretRules: []InterpretRuleSnapshot{{
			Min: 0, Max: 5, RiskLevel: "low", Conclusion: "低", Suggestion: "观察",
		}},
	}

	canonical := original.Canonical()
	domainFactor := canonical.Factor()
	got := FactorFromDomainFactor(domainFactor)

	if got.Code != original.Code ||
		got.Title != original.Title ||
		!got.IsTotalScore ||
		got.QuestionCodes[0] != "q1" ||
		got.ScoringStrategy != original.ScoringStrategy ||
		got.ScoringParams.CntOptionContents[1] != "no" ||
		*got.MaxScore != maxScore ||
		got.InterpretRules[0].RiskLevel != "low" {
		t.Fatalf("domain canonical round trip = %#v, want execution shape %#v", got, original)
	}
}
