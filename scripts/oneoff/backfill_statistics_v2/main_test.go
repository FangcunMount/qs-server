package main

import (
	"testing"
	"time"
)

func TestSplitWindowsUsesInclusiveShanghaiDates(t *testing.T) {
	from, _ := parseShanghaiDate("2026-01-29")
	to, _ := parseShanghaiDate("2026-02-05")
	windows := splitWindows(from, to, 3)
	if len(windows) != 3 {
		t.Fatalf("windows=%d, want 3", len(windows))
	}
	want := [][2]string{{"2026-01-29", "2026-01-31"}, {"2026-02-01", "2026-02-03"}, {"2026-02-04", "2026-02-05"}}
	for index, window := range windows {
		got := [2]string{window.From.Format(dateLayout), window.To.Format(dateLayout)}
		if got != want[index] {
			t.Fatalf("window[%d]=%v, want %v", index, got, want[index])
		}
		if window.From.Location().String() != "Asia/Shanghai" || window.To.Location().String() != "Asia/Shanghai" {
			t.Fatal("window must preserve Asia/Shanghai")
		}
	}
}

func TestOptionsRejectsUnconfirmedWriteAndLargeWindow(t *testing.T) {
	location, _ := time.LoadLocation("Asia/Shanghai")
	base := options{BaseURL: "http://localhost", Token: "secret", OrgIDs: []int64{1}, From: time.Date(2026, 1, 1, 0, 0, 0, 0, location), To: time.Date(2026, 1, 2, 0, 0, 0, 0, location), WindowDays: 7, Reason: "backfill"}
	if err := base.validate(); err == nil {
		t.Fatal("write mode without confirmation must fail")
	}
	base.Confirm = true
	base.WindowDays = 32
	if err := base.validate(); err == nil {
		t.Fatal("window larger than 31 days must fail")
	}
}

func TestParseOrgIDsDeduplicates(t *testing.T) {
	got, err := parseOrgIDs("3, 1,3")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != 3 || got[1] != 1 {
		t.Fatalf("got %v", got)
	}
}
