package main

import "testing"

func TestBuildWeightedAssignmentTargetPool(t *testing.T) {
	targets := make([]clinicianAssignmentTarget, 0, 12)
	for i := 0; i < 12; i++ {
		targets = append(targets, clinicianAssignmentTarget{ID: string(rune('A' + i))})
	}

	pool := buildWeightedAssignmentTargetPool(TesteeAssignmentConfig{
		FocusTargetCount: 10,
		FocusTargetRatio: FlexFloat(0.85),
	}, targets)

	if len(pool) <= len(targets) {
		t.Fatalf("expected weighted pool larger than raw targets, got %d", len(pool))
	}

	focusCount := 0
	otherCount := 0
	for _, item := range pool {
		if item.ID >= "A" && item.ID <= "J" {
			focusCount++
		} else {
			otherCount++
		}
	}
	if focusCount <= otherCount {
		t.Fatalf("expected focus targets to dominate weighted pool, focus=%d other=%d", focusCount, otherCount)
	}
}
