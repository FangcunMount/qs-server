package scorerange

import "testing"

func TestBoundContains(t *testing.T) {
	t.Parallel()

	halfOpen := Bound{Min: 40, Max: 60}
	if halfOpen.Contains(39.9) || !halfOpen.Contains(40) || !halfOpen.Contains(59.9) || halfOpen.Contains(60) {
		t.Fatalf("half-open mismatch")
	}

	inclusive := Bound{Min: 60, Max: 100, MaxInclusive: true}
	if !inclusive.Contains(60) || !inclusive.Contains(100) || inclusive.Contains(100.1) {
		t.Fatalf("max-inclusive mismatch")
	}

	unbounded := Bound{Min: 90, UnboundedMax: true}
	if !unbounded.Contains(90) || !unbounded.Contains(1e9) || unbounded.Contains(89.9) {
		t.Fatalf("unbounded mismatch")
	}
}

func TestMatchBoundsLegacyLastInclusive(t *testing.T) {
	t.Parallel()

	bounds := []Bound{{Min: 0, Max: 60}, {Min: 60, Max: 100}}
	index, ok := MatchBounds(100, bounds)
	if !ok || index != 1 {
		t.Fatalf("legacy last inclusive: index=%d ok=%v", index, ok)
	}
}

func TestRangesOverlapAndGap(t *testing.T) {
	t.Parallel()

	a := Bound{Min: 0, Max: 60}
	b := Bound{Min: 60, Max: 100, MaxInclusive: true}
	if RangesOverlap(a, b) || HasCoverageGap(a, b) {
		t.Fatal("adjacent ranges must neither overlap nor gap")
	}
}
