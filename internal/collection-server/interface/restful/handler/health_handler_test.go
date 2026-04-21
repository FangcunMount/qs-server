package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/cacheobservability"
	"github.com/gin-gonic/gin"
)

func TestHealthHandlerReadyReturnsServiceUnavailableWhenRedisDegraded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	registry := cacheobservability.NewFamilyStatusRegistry("collection-server")
	registry.Update(cacheobservability.FamilyStatus{
		Component: "collection-server",
		Family:    "ops_runtime",
		Profile:   "ops_runtime",
		Available: false,
		Degraded:  true,
		Mode:      cacheobservability.FamilyModeDegraded,
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
	registry := cacheobservability.NewFamilyStatusRegistry("collection-server")
	registry.Update(cacheobservability.FamilyStatus{
		Component: "collection-server",
		Family:    "lock_lease",
		Profile:   "lock_cache",
		Available: true,
		Mode:      cacheobservability.FamilyModeNamedProfile,
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
