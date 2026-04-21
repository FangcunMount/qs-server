package cacheobservability

import "testing"

func TestSnapshotForComponentSummarizesReadiness(t *testing.T) {
	registry := NewFamilyStatusRegistry("worker")
	registry.Update(FamilyStatus{
		Component: "worker",
		Family:    "lock_lease",
		Profile:   "lock_cache",
		Available: true,
		Mode:      FamilyModeNamedProfile,
	})
	registry.Update(FamilyStatus{
		Component: "worker",
		Family:    "ops_runtime",
		Profile:   "ops_runtime",
		Available: false,
		Degraded:  true,
		Mode:      FamilyModeDegraded,
		LastError: "redis unavailable",
	})

	got := SnapshotForComponent("worker", registry)
	if got.Component != "worker" {
		t.Fatalf("Component = %q, want worker", got.Component)
	}
	if got.Summary.FamilyTotal != 2 {
		t.Fatalf("FamilyTotal = %d, want 2", got.Summary.FamilyTotal)
	}
	if got.Summary.AvailableCount != 1 {
		t.Fatalf("AvailableCount = %d, want 1", got.Summary.AvailableCount)
	}
	if got.Summary.DegradedCount != 1 {
		t.Fatalf("DegradedCount = %d, want 1", got.Summary.DegradedCount)
	}
	if got.Summary.UnavailableCount != 1 {
		t.Fatalf("UnavailableCount = %d, want 1", got.Summary.UnavailableCount)
	}
	if got.Summary.Ready {
		t.Fatal("Ready = true, want false")
	}
}

func TestSummarizeFamiliesReadyWhenAllFamiliesAvailable(t *testing.T) {
	got := SummarizeFamilies([]FamilyStatus{
		{Component: "collection-server", Family: "ops_runtime", Available: true},
		{Component: "collection-server", Family: "lock_lease", Available: true},
	})
	if !got.Ready {
		t.Fatal("Ready = false, want true")
	}
	if got.AvailableCount != 2 {
		t.Fatalf("AvailableCount = %d, want 2", got.AvailableCount)
	}
}
