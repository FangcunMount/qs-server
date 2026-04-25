package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	cachegov "github.com/FangcunMount/qs-server/internal/apiserver/application/cachegovernance"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachetarget"
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/middleware"
	"github.com/gin-gonic/gin"
)

type fakeManualWarmupCoordinator struct {
	lastRequest cachegov.ManualWarmupRequest
	result      *cachegov.ManualWarmupResult
	err         error
}

func (f *fakeManualWarmupCoordinator) WarmStartup(context.Context) error { return nil }

func (f *fakeManualWarmupCoordinator) HandleScalePublished(context.Context, string) error { return nil }

func (f *fakeManualWarmupCoordinator) HandleQuestionnairePublished(context.Context, string, string) error {
	return nil
}

func (f *fakeManualWarmupCoordinator) HandleStatisticsSync(context.Context, int64) error { return nil }

func (f *fakeManualWarmupCoordinator) HandleRepairComplete(context.Context, cachegov.RepairCompleteRequest) error {
	return nil
}

func (f *fakeManualWarmupCoordinator) HandleManualWarmup(_ context.Context, req cachegov.ManualWarmupRequest) (*cachegov.ManualWarmupResult, error) {
	f.lastRequest = req
	if f.err != nil {
		return nil, f.err
	}
	if f.result != nil {
		return f.result, nil
	}
	return &cachegov.ManualWarmupResult{
		Trigger:    "manual",
		StartedAt:  time.Unix(1, 0),
		FinishedAt: time.Unix(2, 0),
		Summary: cachegov.ManualWarmupSummary{
			TargetCount: 1,
			OkCount:     1,
			Result:      "ok",
		},
		Items: []cachegov.ManualWarmupItemResult{
			{
				Family: string(cachetarget.NewStaticScaleWarmupTarget("S-001").Family),
				Kind:   cachetarget.WarmupKindStaticScale,
				Scope:  cachetarget.NewStaticScaleWarmupTarget("S-001").Scope,
				Status: cachegov.ManualWarmupItemStatusOK,
			},
		},
	}, nil
}

func (f *fakeManualWarmupCoordinator) Snapshot() cachegov.WarmupStatusSnapshot {
	return cachegov.WarmupStatusSnapshot{}
}

func TestCacheGovernanceStatusReturnsNormalizedEmptySnapshotWhenServiceUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/internal/v1/cache/governance/status", nil)

	handler := &StatisticsHandler{}
	handler.CacheGovernanceStatus(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var payload struct {
		Code int `json:"code"`
		Data struct {
			GeneratedAt string `json:"generated_at"`
			Component   string `json:"component"`
			Summary     struct {
				FamilyTotal      int  `json:"family_total"`
				AvailableCount   int  `json:"available_count"`
				DegradedCount    int  `json:"degraded_count"`
				UnavailableCount int  `json:"unavailable_count"`
				Ready            bool `json:"ready"`
			} `json:"summary"`
			Families []interface{} `json:"families"`
			Warmup   struct {
				Enabled bool `json:"enabled"`
			} `json:"warmup"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload.Code != 0 {
		t.Fatalf("code = %d, want 0", payload.Code)
	}
	if payload.Data.GeneratedAt == "" {
		t.Fatal("generated_at is empty")
	}
	if payload.Data.Component != "apiserver" {
		t.Fatalf("component = %q, want apiserver", payload.Data.Component)
	}
	if payload.Data.Summary.FamilyTotal != 0 {
		t.Fatalf("summary.family_total = %d, want 0", payload.Data.Summary.FamilyTotal)
	}
	if !payload.Data.Summary.Ready {
		t.Fatal("summary.ready = false, want true")
	}
	if len(payload.Data.Families) != 0 {
		t.Fatalf("families len = %d, want 0", len(payload.Data.Families))
	}
	if payload.Data.Warmup.Enabled {
		t.Fatal("warmup.enabled = true, want false")
	}
}

func TestWarmupTargetsReturnsManualWarmupResult(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"targets":[{"kind":"static.scale","scope":"scale:S-001"},{"kind":"query.stats_system","scope":"org:1"}]}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/internal/v1/cache/governance/warmup-targets", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(middleware.OrgIDKey, uint64(1))

	coord := &fakeManualWarmupCoordinator{
		result: &cachegov.ManualWarmupResult{
			Trigger:    "manual",
			StartedAt:  time.Unix(10, 0),
			FinishedAt: time.Unix(12, 0),
			Summary: cachegov.ManualWarmupSummary{
				TargetCount:  2,
				OkCount:      1,
				SkippedCount: 1,
				Result:       "partial",
			},
			Items: []cachegov.ManualWarmupItemResult{
				{Family: "static_meta", Kind: "static.scale", Scope: "scale:s-001", Status: cachegov.ManualWarmupItemStatusOK},
				{Family: "query_result", Kind: "query.stats_system", Scope: "org:1", Status: cachegov.ManualWarmupItemStatusSkipped, Message: "该缓存族未开启预热"},
			},
		},
	}
	handler := &StatisticsHandler{warmupCoordinator: coord}
	handler.WarmupTargets(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var payload struct {
		Code int                         `json:"code"`
		Data cachegov.ManualWarmupResult `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload.Code != 0 {
		t.Fatalf("code = %d, want 0", payload.Code)
	}
	if payload.Data.Trigger != "manual" {
		t.Fatalf("trigger = %q, want manual", payload.Data.Trigger)
	}
	if payload.Data.Summary.TargetCount != 2 {
		t.Fatalf("target_count = %d, want 2", payload.Data.Summary.TargetCount)
	}
	if len(coord.lastRequest.Targets) != 2 {
		t.Fatalf("coordinator request targets len = %d, want 2", len(coord.lastRequest.Targets))
	}
}

func TestWarmupTargetsRejectsCrossOrgQueryTarget(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"targets":[{"kind":"query.stats_system","scope":"org:2"}]}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/internal/v1/cache/governance/warmup-targets", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(middleware.OrgIDKey, uint64(1))

	handler := &StatisticsHandler{warmupCoordinator: &fakeManualWarmupCoordinator{}}
	handler.WarmupTargets(c)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	var payload struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload.Code == 0 {
		t.Fatal("expected non-zero error code")
	}
}

func TestWarmupTargetsRejectsInvalidScope(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"targets":[{"kind":"static.scale","scope":"bad"}]}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/internal/v1/cache/governance/warmup-targets", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(middleware.OrgIDKey, uint64(1))

	handler := &StatisticsHandler{warmupCoordinator: &fakeManualWarmupCoordinator{}}
	handler.WarmupTargets(c)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestWarmupTargetsRejectsInvalidKind(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"targets":[{"kind":"bad.kind","scope":"x"}]}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/internal/v1/cache/governance/warmup-targets", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(middleware.OrgIDKey, uint64(1))

	handler := &StatisticsHandler{warmupCoordinator: &fakeManualWarmupCoordinator{}}
	handler.WarmupTargets(c)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
