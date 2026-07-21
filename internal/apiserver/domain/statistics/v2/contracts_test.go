package v2

import (
	"testing"
	"time"
)

func TestDefaultWindowUsesShanghaiCompleteDays(t *testing.T) {
	now := time.Date(2026, 7, 21, 16, 30, 0, 0, time.UTC) // Shanghai next day 00:30
	window, asOf := DefaultWindow(now, 7)
	if got := window.To.In(Shanghai).Format(time.RFC3339); got != "2026-07-22T00:00:00+08:00" {
		t.Fatalf("to=%s", got)
	}
	if got := asOf.Format("2006-01-02"); got != "2026-07-21" {
		t.Fatalf("as_of=%s", got)
	}
}

func TestRunModeValidate(t *testing.T) {
	for _, mode := range []RunMode{RunModeValidate, RunModeRepair, RunModePublish} {
		if err := mode.Validate(); err != nil {
			t.Fatalf("mode %q: %v", mode, err)
		}
	}
	if err := RunMode("unknown").Validate(); err == nil {
		t.Fatal("expected unknown run mode to fail")
	}
}

func TestDefaultWindowIgnoresProcessLocalTimeZoneAtYearBoundary(t *testing.T) {
	original := time.Local
	time.Local = time.FixedZone("host-west", -7*60*60)
	t.Cleanup(func() { time.Local = original })

	now := time.Date(2025, 12, 31, 16, 30, 0, 0, time.UTC) // Shanghai 2026-01-01 00:30
	window, asOf := DefaultWindow(now, 7)
	if got := window.To.Format(time.RFC3339); got != "2026-01-01T00:00:00+08:00" {
		t.Fatalf("to=%s", got)
	}
	if got := asOf.Format("2006-01-02"); got != "2025-12-31" {
		t.Fatalf("as_of=%s", got)
	}
}

func TestBusinessDateUsesShanghaiAtMonthBoundary(t *testing.T) {
	instant := time.Date(2026, 1, 31, 16, 1, 0, 0, time.UTC)
	if got := BusinessDate(instant).Format("2006-01-02 MST"); got != "2026-02-01 CST" {
		t.Fatalf("business_date=%s", got)
	}
}
