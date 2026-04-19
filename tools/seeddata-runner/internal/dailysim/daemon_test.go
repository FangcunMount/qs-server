package dailysim

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

func TestSelectDailySimulationClinicianIDsForRunStableSubset(t *testing.T) {
	clinicianIDs := []string{"c1", "c2", "c3", "c4", "c5", "c6"}
	cfg := DailySimulationConfig{
		FocusCliniciansPerRunMin: 3,
		FocusCliniciansPerRunMax: 5,
	}
	runDate := time.Date(2026, 4, 17, 0, 0, 0, 0, time.Local)

	first, err := selectDailySimulationClinicianIDsForRun(clinicianIDs, cfg, runDate)
	if err != nil {
		t.Fatalf("selectDailySimulationClinicianIDsForRun returned error: %v", err)
	}
	second, err := selectDailySimulationClinicianIDsForRun(clinicianIDs, cfg, runDate)
	if err != nil {
		t.Fatalf("selectDailySimulationClinicianIDsForRun returned error on second call: %v", err)
	}

	if len(first) != len(second) {
		t.Fatalf("expected stable clinician count, got %d and %d", len(first), len(second))
	}
	if len(first) < 3 || len(first) > 5 {
		t.Fatalf("expected selected clinician count within [3,5], got %d", len(first))
	}
	for idx := range first {
		if first[idx] != second[idx] {
			t.Fatalf("expected stable clinician subset order, mismatch at %d: %s vs %s", idx, first[idx], second[idx])
		}
	}
	seen := make(map[string]struct{}, len(first))
	for _, id := range first {
		if _, exists := seen[id]; exists {
			t.Fatalf("expected unique clinician ids, got duplicate %s", id)
		}
		seen[id] = struct{}{}
	}
}

func TestSelectDailySimulationPlanIDStable(t *testing.T) {
	cfg := DailySimulationConfig{
		PlanIDs: []FlexibleID{"614333603412718126", "614187067651404334"},
	}
	runDate := time.Date(2026, 4, 19, 0, 0, 0, 0, time.Local)

	first := selectDailySimulationPlanID(cfg, runDate, 7)
	second := selectDailySimulationPlanID(cfg, runDate, 7)
	if first != second {
		t.Fatalf("expected stable plan selection, got %q and %q", first, second)
	}
	if first != "614333603412718126" && first != "614187067651404334" {
		t.Fatalf("unexpected selected plan id %q", first)
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
