package main

import (
	"testing"
	"time"
)

func TestResolveDailySimulationBatchCountStableRange(t *testing.T) {
	cfg := DailySimulationConfig{
		CountMin: 10,
		CountMax: 50,
	}
	runDate := time.Date(2026, 4, 17, 0, 0, 0, 0, time.Local)

	first, err := resolveDailySimulationBatchCount(cfg, runDate)
	if err != nil {
		t.Fatalf("resolveDailySimulationBatchCount returned error: %v", err)
	}
	second, err := resolveDailySimulationBatchCount(cfg, runDate)
	if err != nil {
		t.Fatalf("resolveDailySimulationBatchCount returned error on second call: %v", err)
	}
	if first != second {
		t.Fatalf("expected stable count for same date, got %d and %d", first, second)
	}
	if first < 10 || first > 50 {
		t.Fatalf("expected count within [10,50], got %d", first)
	}
}

func TestSelectDailySimulationCliniciansForRunStableSubset(t *testing.T) {
	clinicians := []*ClinicianResponse{
		{ID: "c1", Name: "A"},
		{ID: "c2", Name: "B"},
		{ID: "c3", Name: "C"},
		{ID: "c4", Name: "D"},
		{ID: "c5", Name: "E"},
		{ID: "c6", Name: "F"},
	}
	cfg := DailySimulationConfig{
		FocusCliniciansPerRunMin: 3,
		FocusCliniciansPerRunMax: 5,
	}
	runDate := time.Date(2026, 4, 17, 0, 0, 0, 0, time.Local)

	first, err := selectDailySimulationCliniciansForRun(clinicians, cfg, runDate)
	if err != nil {
		t.Fatalf("selectDailySimulationCliniciansForRun returned error: %v", err)
	}
	second, err := selectDailySimulationCliniciansForRun(clinicians, cfg, runDate)
	if err != nil {
		t.Fatalf("selectDailySimulationCliniciansForRun returned error on second call: %v", err)
	}

	if len(first) != len(second) {
		t.Fatalf("expected stable clinician count, got %d and %d", len(first), len(second))
	}
	if len(first) < 3 || len(first) > 5 {
		t.Fatalf("expected selected clinician count within [3,5], got %d", len(first))
	}
	for idx := range first {
		if first[idx].ID != second[idx].ID {
			t.Fatalf("expected stable clinician subset order, mismatch at %d: %s vs %s", idx, first[idx].ID, second[idx].ID)
		}
	}
	seen := make(map[string]struct{}, len(first))
	for _, item := range first {
		if _, exists := seen[item.ID]; exists {
			t.Fatalf("expected unique clinician ids, got duplicate %s", item.ID)
		}
		seen[item.ID] = struct{}{}
	}
}

func TestNextDailySimulationDaemonRun(t *testing.T) {
	clock := dailySimulationRunClock{Hour: 10, Minute: 0}

	nowBefore := time.Date(2026, 4, 17, 9, 15, 0, 0, time.Local)
	runDate, wait := nextDailySimulationDaemonRun(nowBefore, clock, "")
	if got, want := runDate.Format("2006-01-02"), "2026-04-17"; got != want {
		t.Fatalf("expected run date %s before scheduled time, got %s", want, got)
	}
	if wait <= 0 {
		t.Fatalf("expected positive wait before scheduled time, got %s", wait)
	}

	nowAfter := time.Date(2026, 4, 17, 10, 5, 0, 0, time.Local)
	runDate, wait = nextDailySimulationDaemonRun(nowAfter, clock, "")
	if got, want := runDate.Format("2006-01-02"), "2026-04-17"; got != want {
		t.Fatalf("expected same-day run date after scheduled time, got %s", got)
	}
	if wait != 0 {
		t.Fatalf("expected zero wait after scheduled time when not yet successful, got %s", wait)
	}

	runDate, wait = nextDailySimulationDaemonRun(nowAfter, clock, "2026-04-17")
	if got, want := runDate.Format("2006-01-02"), "2026-04-18"; got != want {
		t.Fatalf("expected next-day run date after success, got %s", got)
	}
	if wait <= 0 {
		t.Fatalf("expected positive wait after same-day success, got %s", wait)
	}
}
