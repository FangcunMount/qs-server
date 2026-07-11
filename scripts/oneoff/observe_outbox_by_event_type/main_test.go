package main

import "testing"

func TestEvaluateGatePassWhenNoLegacyBacklogOrWrites(t *testing.T) {
	rep := report{RecentDays: 7}
	if got := evaluateGate(rep); got.Status != "PASS" {
		t.Fatalf("gate = %s, want PASS", got.Status)
	}
}

func TestEvaluateGateWarnOnLegacyBacklog(t *testing.T) {
	rep := report{RecentDays: 7, LegacyUnfinished: 3}
	if got := evaluateGate(rep); got.Status != "WARN" {
		t.Fatalf("gate = %s, want WARN", got.Status)
	}
}

func TestEvaluateGateWarnOnLegacyRecentWrites(t *testing.T) {
	rep := report{RecentDays: 7, LegacyRecent: 1}
	if got := evaluateGate(rep); got.Status != "WARN" {
		t.Fatalf("gate = %s, want WARN", got.Status)
	}
}

func TestLegacyEventTypesAreEmptyAfterEventContractCutover(t *testing.T) {
	if len(legacyEventTypes) != 0 {
		t.Fatalf("legacy event types = %#v", legacyEventTypes)
	}
}

func TestIsLegacyEventTypeIsAlwaysFalseAfterCutover(t *testing.T) {
	if isLegacyEventType("retired.event") {
		t.Fatal("no retired event types should remain in the active observer")
	}
}
