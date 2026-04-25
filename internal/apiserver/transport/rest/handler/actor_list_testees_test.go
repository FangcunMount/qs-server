package handler

import (
	"testing"
	"time"
)

func TestParseInclusiveLocalDateRange(t *testing.T) {
	start, end, err := parseInclusiveLocalDateRange("2026-04-15", "2026-04-17")
	if err != nil {
		t.Fatalf("parseInclusiveLocalDateRange returned error: %v", err)
	}
	if start == nil || end == nil {
		t.Fatalf("expected both start and end to be set")
	}
	if got, want := start.Format("2006-01-02 15:04:05"), "2026-04-15 00:00:00"; got != want {
		t.Fatalf("start = %s, want %s", got, want)
	}
	if got, want := end.Format("2006-01-02 15:04:05"), "2026-04-18 00:00:00"; got != want {
		t.Fatalf("end = %s, want %s", got, want)
	}
}

func TestParseInclusiveLocalDateRangeRejectsInvalidOrder(t *testing.T) {
	if _, _, err := parseInclusiveLocalDateRange("2026-04-18", "2026-04-17"); err == nil {
		t.Fatalf("expected invalid date range to fail")
	}
}

func TestCreatedAtInRangeUsesExclusiveEndBoundary(t *testing.T) {
	start, end, err := parseInclusiveLocalDateRange("2026-04-15", "2026-04-17")
	if err != nil {
		t.Fatalf("parseInclusiveLocalDateRange returned error: %v", err)
	}

	inRange := time.Date(2026, 4, 17, 23, 59, 59, 0, time.Local)
	if !createdAtInRange(inRange, start, end) {
		t.Fatalf("expected timestamp at end-date 23:59:59 to be included")
	}

	outOfRange := time.Date(2026, 4, 18, 0, 0, 0, 0, time.Local)
	if createdAtInRange(outOfRange, start, end) {
		t.Fatalf("expected timestamp at next day 00:00:00 to be excluded")
	}
}
