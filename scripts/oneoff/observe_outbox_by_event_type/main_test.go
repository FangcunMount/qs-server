package main

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
)

func TestEvaluateGatePassWhenNoLegacyBacklogOrWrites(t *testing.T) {
	rep := report{RecentDays: 7}
	got := evaluateGate(rep)
	if got.Status != "PASS" {
		t.Fatalf("gate = %s, want PASS", got.Status)
	}
}

func TestEvaluateGateWarnOnLegacyBacklog(t *testing.T) {
	rep := report{RecentDays: 7, LegacyUnfinished: 3}
	got := evaluateGate(rep)
	if got.Status != "WARN" {
		t.Fatalf("gate = %s, want WARN", got.Status)
	}
}

func TestEvaluateGateWarnOnLegacyRecentWrites(t *testing.T) {
	rep := report{RecentDays: 7, LegacyRecent: 1}
	got := evaluateGate(rep)
	if got.Status != "WARN" {
		t.Fatalf("gate = %s, want WARN", got.Status)
	}
}

func TestLegacyEventTypesMatchDeprecatedWireNames(t *testing.T) {
	if legacyEventTypes[0] != eventcatalog.AssessmentInterpretedWireV2 {
		t.Fatalf("deprecated wire = %q", legacyEventTypes[0])
	}
	if legacyEventTypes[1] != eventcatalog.ReportGeneratedWireV2 {
		t.Fatalf("deprecated wire = %q", legacyEventTypes[1])
	}
}

func TestIsLegacyEventTypeMatchesDeprecatedWireOnly(t *testing.T) {
	if !isLegacyEventType(eventcatalog.AssessmentInterpretedWireV2) {
		t.Fatal("assessment.interpreted.v2 should be deprecated wire backlog")
	}
	if isLegacyEventType(eventcatalog.AssessmentInterpreted) {
		t.Fatal("assessment.interpreted is canonical outcome wire")
	}
}
