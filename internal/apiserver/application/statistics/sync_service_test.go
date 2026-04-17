package statistics

import (
	"testing"
	"time"
)

func TestNormalizeDailyWindowDefaultsToRepairWindow(t *testing.T) {
	service := &syncService{repairWindowDays: 7}
	now := time.Date(2026, 4, 17, 10, 0, 0, 0, time.Local)

	start, end, err := service.normalizeDailyWindow(now, SyncDailyOptions{})
	if err != nil {
		t.Fatalf("normalizeDailyWindow returned error: %v", err)
	}
	if want := time.Date(2026, 4, 10, 0, 0, 0, 0, time.Local); !start.Equal(want) {
		t.Fatalf("unexpected start: got %s want %s", start, want)
	}
	if want := time.Date(2026, 4, 17, 0, 0, 0, 0, time.Local); !end.Equal(want) {
		t.Fatalf("unexpected end: got %s want %s", end, want)
	}
}

func TestNormalizeDailyWindowRejectsPartialRange(t *testing.T) {
	service := &syncService{repairWindowDays: 7}
	start := time.Date(2026, 4, 10, 0, 0, 0, 0, time.Local)

	if _, _, err := service.normalizeDailyWindow(time.Now(), SyncDailyOptions{StartDate: &start}); err == nil {
		t.Fatalf("expected error for partial date range")
	}
}
