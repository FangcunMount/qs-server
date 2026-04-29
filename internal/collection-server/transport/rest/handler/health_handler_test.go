package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/cachegovernance/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/resilienceplane"
	"github.com/gin-gonic/gin"
)

func TestHealthHandlerReadyReturnsServiceUnavailableWhenRedisDegraded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	registry := observability.NewFamilyStatusRegistry("collection-server")
	registry.Update(observability.FamilyStatus{
		Component: "collection-server",
		Family:    "ops_runtime",
		Profile:   "ops_runtime",
		Available: false,
		Degraded:  true,
		Mode:      observability.FamilyModeDegraded,
		LastError: "redis unavailable",
	})
	handler := NewHealthHandler("collection-server", "2.0.0", registry)

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	ctx.Request = req

	handler.Ready(ctx)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}

	var payload struct {
		Data struct {
			Status string `json:"status"`
			Redis  struct {
				Summary struct {
					Ready bool `json:"ready"`
				} `json:"summary"`
			} `json:"redis"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload.Data.Status != "degraded" {
		t.Fatalf("status payload = %q, want degraded", payload.Data.Status)
	}
	if payload.Data.Redis.Summary.Ready {
		t.Fatal("redis summary ready = true, want false")
	}
}

func TestHealthHandlerRedisFamiliesReturnsSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	registry := observability.NewFamilyStatusRegistry("collection-server")
	registry.Update(observability.FamilyStatus{
		Component: "collection-server",
		Family:    "lock_lease",
		Profile:   "lock_cache",
		Available: true,
		Mode:      observability.FamilyModeNamedProfile,
	})
	handler := NewHealthHandler("collection-server", "2.0.0", registry)

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodGet, "/governance/redis", nil)
	ctx.Request = req

	handler.RedisFamilies(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var payload struct {
		Data struct {
			Component string `json:"component"`
			Summary   struct {
				Ready bool `json:"ready"`
			} `json:"summary"`
			Families []struct {
				Family string `json:"family"`
			} `json:"families"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload.Data.Component != "collection-server" {
		t.Fatalf("component = %q, want collection-server", payload.Data.Component)
	}
	if !payload.Data.Summary.Ready {
		t.Fatal("summary.ready = false, want true")
	}
	if len(payload.Data.Families) != 1 || payload.Data.Families[0].Family != "lock_lease" {
		t.Fatalf("unexpected families payload: %+v", payload.Data.Families)
	}
}

func TestHealthHandlerResilienceReturnsSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewHealthHandlerWithResilience(
		"collection-server",
		"2.0.0",
		nil,
		func() resilienceplane.RuntimeSnapshot {
			snapshot := resilienceplane.NewRuntimeSnapshot("collection-server", time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC))
			snapshot.Queues = []resilienceplane.QueueSnapshot{{
				Component:         "collection-server",
				Name:              "answersheet_submit",
				Strategy:          "memory_channel",
				Depth:             1,
				Capacity:          10,
				StatusCounts:      map[string]int{"queued": 1},
				LifecycleBoundary: "process_memory_no_drain",
			}}
			return resilienceplane.FinalizeRuntimeSnapshot(snapshot)
		},
	)

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodGet, "/governance/resilience", nil)
	ctx.Request = req

	handler.Resilience(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var payload struct {
		Data struct {
			Component string `json:"component"`
			Summary   struct {
				CapabilityCount int `json:"capability_count"`
			} `json:"summary"`
			Queues []struct {
				Name string `json:"name"`
			} `json:"queues"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload.Data.Component != "collection-server" {
		t.Fatalf("component = %q, want collection-server", payload.Data.Component)
	}
	if payload.Data.Summary.CapabilityCount != 1 {
		t.Fatalf("capability_count = %d, want 1", payload.Data.Summary.CapabilityCount)
	}
	if len(payload.Data.Queues) != 1 || payload.Data.Queues[0].Name != "answersheet_submit" {
		t.Fatalf("queues = %+v, want answersheet_submit", payload.Data.Queues)
	}
}
