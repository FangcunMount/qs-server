package conclusion

import "testing"

func TestScoreRangeBoundContains(t *testing.T) {
	t.Parallel()

	halfOpen := ScoreRangeBound{Min: 40, Max: 60}
	if halfOpen.Contains(39.9) || !halfOpen.Contains(40) || !halfOpen.Contains(59.9) || halfOpen.Contains(60) {
		t.Fatalf("half-open mismatch")
	}

	inclusive := ScoreRangeBound{Min: 60, Max: 100, MaxInclusive: true}
	if !inclusive.Contains(60) || !inclusive.Contains(100) || inclusive.Contains(100.1) {
		t.Fatalf("max-inclusive mismatch")
	}

	unbounded := ScoreRangeBound{Min: 90, UnboundedMax: true}
	if !unbounded.Contains(90) || !unbounded.Contains(1e9) || unbounded.Contains(89.9) {
		t.Fatalf("unbounded mismatch")
	}
}

func TestMatchScoreRangeOutcomesLegacyLastInclusive(t *testing.T) {
	t.Parallel()

	rules := []ScoreRangeOutcome{
		{MinScore: 0, MaxScore: 60, OutcomeCode: "low"},
		{MinScore: 60, MaxScore: 100, OutcomeCode: "high"},
	}
	got, ok := MatchScoreRangeOutcomes(100, rules)
	if !ok || got.OutcomeCode != "high" {
		t.Fatalf("legacy last inclusive: got=%#v ok=%v", got, ok)
	}
	got, ok = MatchScoreRangeOutcomes(60, rules)
	if !ok || got.OutcomeCode != "high" {
		t.Fatalf("boundary 60 should hit second range: got=%#v ok=%v", got, ok)
	}
}

func TestMatchScoreRangeOutcomesExplicitNoLegacy(t *testing.T) {
	t.Parallel()

	rules := []ScoreRangeOutcome{
		{MinScore: 0, MaxScore: 60, OutcomeCode: "low"},
		{MinScore: 60, MaxScore: 100, OutcomeCode: "high", MaxInclusive: true},
	}
	if _, ok := MatchScoreRangeOutcomes(100, rules); !ok {
		t.Fatal("expected max inclusive hit")
	}
	onlyHalf := []ScoreRangeOutcome{
		{MinScore: 0, MaxScore: 100, OutcomeCode: "all", MaxInclusive: true},
	}
	if _, ok := MatchScoreRangeOutcomes(100, onlyHalf); !ok {
		t.Fatal("single max-inclusive rule should match 100")
	}
}

func TestRangesOverlapAndGap(t *testing.T) {
	t.Parallel()

	a := ScoreRangeBound{Min: 0, Max: 60}
	b := ScoreRangeBound{Min: 60, Max: 100, MaxInclusive: true}
	if RangesOverlap(a, b) {
		t.Fatal("adjacent half-open then inclusive must not overlap")
	}
	if HasCoverageGap(a, b) {
		t.Fatal("adjacent ranges must not report gap")
	}

	c := ScoreRangeBound{Min: 0, Max: 40}
	d := ScoreRangeBound{Min: 50, Max: 100, MaxInclusive: true}
	if !HasCoverageGap(c, d) {
		t.Fatal("expected gap")
	}

	e := ScoreRangeBound{Min: 0, Max: 60}
	f := ScoreRangeBound{Min: 50, Max: 100, MaxInclusive: true}
	if !RangesOverlap(e, f) {
		t.Fatal("expected overlap")
	}
}
