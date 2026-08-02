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
	base := options{BaseURL: "http://localhost", Token: "secret", OrgIDs: []int64{1}, From: time.Date(2026, 1, 1, 0, 0, 0, 0, location), To: time.Date(2026, 1, 2, 0, 0, 0, 0, location), WindowDays: 7, RequestTimeout: defaultRequestTimeout, Reason: "backfill"}
	if err := base.validate(); err == nil {
		t.Fatal("write mode without confirmation must fail")
	}
	base.Confirm = true
	base.WindowDays = 32
	if err := base.validate(); err == nil {
		t.Fatal("window larger than 31 days must fail")
	}
}

func TestOptionsRejectsInvalidTimeoutAndMode(t *testing.T) {
	location, _ := time.LoadLocation("Asia/Shanghai")
	base := options{
		BaseURL: "http://localhost", Token: "secret", OrgIDs: []int64{1},
		From: time.Date(2026, 1, 1, 0, 0, 0, 0, location), To: time.Date(2026, 1, 2, 0, 0, 0, 0, location),
		WindowDays: 7, RequestTimeout: defaultRequestTimeout, Reason: "backfill", Mode: "repair", Confirm: true,
	}
	base.RequestTimeout = 0
	if err := base.validate(); err == nil || !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("err=%v", err)
	}
	base.RequestTimeout = defaultRequestTimeout
	base.Mode = "unsupported"
	if err := base.validate(); err == nil || !strings.Contains(err.Error(), "mode") {
		t.Fatalf("err=%v", err)
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

func TestRunValidateFailsWhenFactsWouldStillBeInserted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"data":{"id":12,"mode":"validate","status":"succeeded","stage":"completed","fact_counts":{"access.inserted":0,"access.conflict":0,"plan.inserted":1,"plan.conflict":0,"assessment.inserted":0,"assessment.conflict":0}}}`))
	}))
	defer server.Close()

	err := run([]string{
		"--base-url", server.URL,
		"--token", "secret",
		"--org-ids", "7",
		"--from", "2026-01-01",
		"--to", "2026-01-01",
		"--validate-only",
	}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "plan.inserted=1") {
		t.Fatalf("err=%v", err)
	}
}

func TestValidationCompletenessRequiresCanonicalCollectorCounts(t *testing.T) {
	err := validateRunCompleteness(runResult{FactCounts: map[string]int64{
		"access.inserted": 0, "access.conflict": 0,
		"plan.inserted": 0, "plan.conflict": 0,
	}})
	if err == nil || !strings.Contains(err.Error(), "assessment.inserted") {
		t.Fatalf("err=%v", err)
	}
}

func TestExecuteRunWithCacheRecoveryResumesDataCommittedPublish(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.Header.Get("X-Org-ID") != "7" || r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("org=%s auth=%s", r.Header.Get("X-Org-ID"), r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/internal/v2/statistics/runs":
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":13,"mode":"publish","status":"data_committed","stage":"publishing_cache"}}`))
		case "/internal/v2/statistics/runs/13/resume-cache":
			var request resumeCacheRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatal(err)
			}
			if !request.Confirm || request.Reason != "approved repair" {
				t.Fatalf("request=%+v", request)
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":13,"mode":"publish","status":"succeeded","stage":"completed","as_of_date":"2026-01-01","cache_generation":3}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	location, _ := time.LoadLocation("Asia/Shanghai")
	result, err := executeRunWithCacheRecovery(server.Client(), options{
		BaseURL: server.URL, Token: "secret", Mode: "publish", Reason: "approved repair", Confirm: true,
	}, 7, dateWindow{
		From: time.Date(2026, 1, 1, 0, 0, 0, 0, location),
		To:   time.Date(2026, 1, 1, 0, 0, 0, 0, location),
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "succeeded" || result.CacheGeneration != 3 {
		t.Fatalf("result=%+v", result)
	}
	want := []string{"/internal/v2/statistics/runs", "/internal/v2/statistics/runs/13/resume-cache"}
	if strings.Join(paths, ",") != strings.Join(want, ",") {
		t.Fatalf("paths=%v want=%v", paths, want)
	}
}
