package apiserver

import (
	"testing"
	"time"
)

func TestParseStatisticsSyncRunAt(t *testing.T) {
	clock, err := parseStatisticsSyncRunAt("00:30")
	if err != nil {
		t.Fatalf("parseStatisticsSyncRunAt returned error: %v", err)
	}
	if clock.hour != 0 || clock.minute != 30 {
		t.Fatalf("unexpected clock: %+v", clock)
	}
}

func TestNextStatisticsSyncRun(t *testing.T) {
	now := time.Date(2026, 4, 17, 0, 29, 0, 0, time.Local)
	next := nextStatisticsSyncRun(now, 0, 30)
	want := time.Date(2026, 4, 17, 0, 30, 0, 0, time.Local)
	if !next.Equal(want) {
		t.Fatalf("unexpected next run: got %s want %s", next, want)
	}

	now = time.Date(2026, 4, 17, 0, 31, 0, 0, time.Local)
	next = nextStatisticsSyncRun(now, 0, 30)
	want = time.Date(2026, 4, 18, 0, 30, 0, 0, time.Local)
	if !next.Equal(want) {
		t.Fatalf("unexpected rolled next run: got %s want %s", next, want)
	}
}
