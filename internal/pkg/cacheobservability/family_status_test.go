package cacheobservability

import (
	"errors"
	"testing"
)

func TestFamilyStatusRegistrySnapshot(t *testing.T) {
	registry := NewFamilyStatusRegistry("apiserver")
	registry.Update(FamilyStatus{
		Family:      "query_result",
		Profile:     "query_cache",
		Available:   true,
		Configured:  true,
		Mode:        FamilyModeNamedProfile,
		AllowWarmup: true,
	})
	registry.Update(FamilyStatus{
		Family:     "meta_hotset",
		Profile:    "meta_cache",
		Available:  false,
		Degraded:   true,
		Configured: true,
		Mode:       FamilyModeDegraded,
		LastError:  "dial tcp timeout",
	})

	snapshot := registry.Snapshot()
	if len(snapshot) != 2 {
		t.Fatalf("Snapshot() len = %d, want 2", len(snapshot))
	}
	if snapshot[0].Family != "meta_hotset" {
		t.Fatalf("Snapshot()[0].Family = %s, want meta_hotset", snapshot[0].Family)
	}
	if snapshot[1].Family != "query_result" {
		t.Fatalf("Snapshot()[1].Family = %s, want query_result", snapshot[1].Family)
	}
	if snapshot[0].LastError == "" {
		t.Fatal("Snapshot()[0].LastError should be preserved")
	}
}

func TestFamilyStatusRegistryRuntimeTransitions(t *testing.T) {
	registry := NewFamilyStatusRegistry("apiserver")
	registry.Update(FamilyStatus{
		Family:     "query_result",
		Profile:    "query_cache",
		Available:  true,
		Configured: true,
		Mode:       FamilyModeNamedProfile,
	})

	registry.RecordFailure("query_result", errors.New("redis down"))
	snapshot := registry.Snapshot()
	if len(snapshot) != 1 {
		t.Fatalf("Snapshot() len = %d, want 1", len(snapshot))
	}
	if !snapshot[0].Degraded || snapshot[0].Available {
		t.Fatalf("runtime failure should mark family degraded, got %+v", snapshot[0])
	}
	if snapshot[0].ConsecutiveFailures != 1 {
		t.Fatalf("ConsecutiveFailures = %d, want 1", snapshot[0].ConsecutiveFailures)
	}
	if snapshot[0].LastFailureAt.IsZero() {
		t.Fatal("LastFailureAt should be populated")
	}

	registry.RecordSuccess("query_result")
	snapshot = registry.Snapshot()
	if snapshot[0].Degraded || !snapshot[0].Available {
		t.Fatalf("runtime success should recover family status, got %+v", snapshot[0])
	}
	if snapshot[0].Mode != FamilyModeNamedProfile {
		t.Fatalf("Mode = %s, want %s", snapshot[0].Mode, FamilyModeNamedProfile)
	}
	if snapshot[0].ConsecutiveFailures != 0 {
		t.Fatalf("ConsecutiveFailures = %d, want 0", snapshot[0].ConsecutiveFailures)
	}
	if snapshot[0].LastSuccessAt.IsZero() {
		t.Fatal("LastSuccessAt should be populated")
	}
}
