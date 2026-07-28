package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHistoricalBackfillWorkflowRepairsValidatesCatchupAndPublishesLatestCompleteDay(t *testing.T) {
	var requests []runRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request runRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		requests = append(requests, request)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"id":1,"status":"succeeded","stage":"completed","as_of_date":"` + request.ToDate + `"}}`))
	}))
	defer server.Close()

	location, _ := time.LoadLocation("Asia/Shanghai")
	cfg := options{
		BaseURL: server.URL, Token: "secret", OrgIDs: []int64{7},
		From:       time.Date(2026, 1, 1, 0, 0, 0, 0, location),
		To:         time.Date(2026, 1, 5, 0, 0, 0, 0, location),
		WindowDays: 3, Reason: "historical backfill", Mode: historicalBackfillMode, Confirm: true,
		Now: func() time.Time { return time.Date(2026, 1, 7, 12, 0, 0, 0, location) },
	}
	var output bytes.Buffer
	if err := executeHistoricalBackfill(server.Client(), cfg, 7, &output); err != nil {
		t.Fatal(err)
	}
	if len(requests) != 7 {
		t.Fatalf("requests=%d, want 7", len(requests))
	}
	want := []struct{ mode, from, to string }{
		{"repair", "2026-01-01", "2026-01-03"},
		{"validate", "2026-01-01", "2026-01-03"},
		{"repair", "2026-01-04", "2026-01-05"},
		{"validate", "2026-01-04", "2026-01-05"},
		{"repair", "2026-01-06", "2026-01-06"},
		{"validate", "2026-01-06", "2026-01-06"},
		{"publish", "2026-01-06", "2026-01-06"},
	}
	for index, expected := range want {
		got := requests[index]
		if got.Mode != expected.mode || got.FromDate != expected.from || got.ToDate != expected.to {
			t.Fatalf("request[%d]=%+v, want %+v", index, got, expected)
		}
		if got.Mode == "validate" && (!got.ValidateOnly || got.Confirm) {
			t.Fatalf("validate request[%d] must be read-only: %+v", index, got)
		}
	}
}

func TestHistoricalBackfillRejectsWhenLatestCompleteDayPrecedesRequestedEnd(t *testing.T) {
	location, _ := time.LoadLocation("Asia/Shanghai")
	cfg := options{
		BaseURL: "http://localhost", Token: "secret", OrgIDs: []int64{7},
		From: time.Date(2026, 1, 1, 0, 0, 0, 0, location), To: time.Date(2026, 1, 5, 0, 0, 0, 0, location),
		WindowDays: 3, Reason: "historical backfill", Mode: historicalBackfillMode, Confirm: true,
		Now: func() time.Time { return time.Date(2026, 1, 5, 12, 0, 0, 0, location) },
	}
	if err := executeHistoricalBackfill(http.DefaultClient, cfg, 7, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "before historical backfill end") {
		t.Fatalf("err=%v", err)
	}
}

func TestHistoricalRangeSplitsIntoNineteenWindows(t *testing.T) {
	from, _ := parseShanghaiDate("2025-01-01")
	to, _ := parseShanghaiDate("2026-07-27")
	windows := splitWindows(from, to, 31)
	if len(windows) != 19 {
		t.Fatalf("windows=%d, want 19", len(windows))
	}
	if windows[len(windows)-1].To.Format(dateLayout) != "2026-07-27" {
		t.Fatalf("last window=%s", windows[len(windows)-1].To.Format(dateLayout))
	}
}

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

func TestExecuteRunSendsScopedRepairRequestAndParsesCounts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/v2/statistics/runs" || r.Header.Get("X-Org-ID") != "7" || r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("path=%s org=%s auth=%s", r.URL.Path, r.Header.Get("X-Org-ID"), r.Header.Get("Authorization"))
		}
		var request runRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.Mode != "repair" || request.FromDate != "2026-01-01" || request.ToDate != "2026-01-07" || !request.Confirm {
			t.Fatalf("request=%+v", request)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"id":9,"mode":"repair","status":"succeeded","stage":"completed","fact_counts":{"assessment.inserted":3}}}`))
	}))
	defer server.Close()

	location, _ := time.LoadLocation("Asia/Shanghai")
	result, err := executeRun(server.Client(), options{BaseURL: server.URL, Token: "secret", Mode: "repair", Reason: "approved", Confirm: true}, 7, dateWindow{
		From: time.Date(2026, 1, 1, 0, 0, 0, 0, location),
		To:   time.Date(2026, 1, 7, 0, 0, 0, 0, location),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ID != 9 || result.FactCounts["assessment.inserted"] != 3 {
		t.Fatalf("result=%+v", result)
	}
}

func TestExecuteRunStopsAtDataCommittedWithResumeGuidance(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"data":{"id":11,"mode":"publish","status":"data_committed","stage":"publishing_cache"}}`))
	}))
	defer server.Close()

	location, _ := time.LoadLocation("Asia/Shanghai")
	_, err := executeRun(server.Client(), options{BaseURL: server.URL, Token: "secret", Mode: "publish", Reason: "approved", Confirm: true}, 7, dateWindow{
		From: time.Date(2026, 1, 1, 0, 0, 0, 0, location),
		To:   time.Date(2026, 1, 1, 0, 0, 0, 0, location),
	})
	if err == nil || !strings.Contains(err.Error(), "resume cache") {
		t.Fatalf("err=%v", err)
	}
}
