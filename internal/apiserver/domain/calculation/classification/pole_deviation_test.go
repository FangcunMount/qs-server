package classification

import "testing"

func TestPoleMaxDeviationUsesOptionScoreSpan(t *testing.T) {
	t.Parallel()
	got := PoleMaxDeviation(0, 24, []AnswerContribution{{
		OptionScores: map[string]float64{"A": 1, "B": 5},
	}})
	// min=1 max=5 threshold=24 → left=23 right=-19 → 23
	if got != 23 {
		t.Fatalf("got %v, want 23", got)
	}
}

func TestPoleMaxDeviationLikertSignPositive(t *testing.T) {
	t.Parallel()
	got := PoleMaxDeviation(0, 24, []AnswerContribution{{Sign: 1}})
	// min=1 max=5 → left=23 right=-19 → 23
	if got != 23 {
		t.Fatalf("got %v, want 23", got)
	}
}

func TestPoleMaxDeviationLikertSignNegative(t *testing.T) {
	t.Parallel()
	got := PoleMaxDeviation(0, 24, []AnswerContribution{{Sign: -1}})
	// min=-5 max=-1 → left=29 right=-25 → 29
	if got != 29 {
		t.Fatalf("got %v, want 29", got)
	}
}

func TestPoleMaxDeviationDefaultThreshold(t *testing.T) {
	t.Parallel()
	withDefault := PoleMaxDeviation(0, 0, []AnswerContribution{{Sign: 1}})
	withExplicit := PoleMaxDeviation(0, 24, []AnswerContribution{{Sign: 1}})
	if withDefault != withExplicit {
		t.Fatalf("default threshold = %v, explicit = %v", withDefault, withExplicit)
	}
}

func TestPoleMaxDeviationIncludesConstant(t *testing.T) {
	t.Parallel()
	got := PoleMaxDeviation(10, 24, []AnswerContribution{{Sign: 1}})
	// min=11 max=15 → left=13 right=-9 → 13
	if got != 13 {
		t.Fatalf("got %v, want 13", got)
	}
}
