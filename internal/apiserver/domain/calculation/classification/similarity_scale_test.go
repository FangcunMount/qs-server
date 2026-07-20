package classification

import "testing"

func TestDualScaleFromScoreSimilarityUnit(t *testing.T) {
	t.Parallel()
	mp, sim := DualScaleFromScore(0.42)
	if mp != 42 || sim != 0.42 {
		t.Fatalf("DualScaleFromScore(0.42) = (%v, %v), want (42, 0.42)", mp, sim)
	}
}

func TestDualScaleFromScoreBoundaryOne(t *testing.T) {
	t.Parallel()
	mp, sim := DualScaleFromScore(1)
	if mp != 100 || sim != 1 {
		t.Fatalf("DualScaleFromScore(1) = (%v, %v), want (100, 1)", mp, sim)
	}
}

func TestDualScaleFromScorePercentUnit(t *testing.T) {
	t.Parallel()
	mp, sim := DualScaleFromScore(85)
	if mp != 85 || sim != 0.85 {
		t.Fatalf("DualScaleFromScore(85) = (%v, %v), want (85, 0.85)", mp, sim)
	}
}

func TestDualScaleFromScoreZero(t *testing.T) {
	t.Parallel()
	mp, sim := DualScaleFromScore(0)
	if mp != 0 || sim != 0 {
		t.Fatalf("DualScaleFromScore(0) = (%v, %v), want (0, 0)", mp, sim)
	}
}

func TestMatchPercentPreferUsesMatchPercent(t *testing.T) {
	t.Parallel()
	if got := MatchPercentPrefer(42, 0.9); got != 42 {
		t.Fatalf("MatchPercentPrefer(42, 0.9) = %v, want 42", got)
	}
}

func TestMatchPercentPreferFallsBackToSimilarity(t *testing.T) {
	t.Parallel()
	if got := MatchPercentPrefer(0, 0.8); got != 80 {
		t.Fatalf("MatchPercentPrefer(0, 0.8) = %v, want 80", got)
	}
}

func TestMatchPercentPreferZeroBoth(t *testing.T) {
	t.Parallel()
	if got := MatchPercentPrefer(0, 0); got != 0 {
		t.Fatalf("MatchPercentPrefer(0, 0) = %v, want 0", got)
	}
}
