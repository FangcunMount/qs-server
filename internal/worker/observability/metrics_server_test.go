package observability

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/cacheobservability"
)

func TestMetricsServerReadyzReturnsServiceUnavailableWhenFamilyDegraded(t *testing.T) {
	registry := cacheobservability.NewFamilyStatusRegistry("worker")
	registry.Update(cacheobservability.FamilyStatus{
		Component: "worker",
		Family:    "lock_lease",
		Profile:   "lock_cache",
		Available: false,
		Degraded:  true,
		Mode:      cacheobservability.FamilyModeDegraded,
		LastError: "redis unavailable",
	})

	server := NewMetricsServerWithGovernance("127.0.0.1", 19091, "worker", registry)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	server.server.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload["status"] != "degraded" {
		t.Fatalf("status payload = %v, want degraded", payload["status"])
	}
}

func TestMetricsServerGovernanceEndpointReturnsSnapshot(t *testing.T) {
	registry := cacheobservability.NewFamilyStatusRegistry("worker")
	registry.Update(cacheobservability.FamilyStatus{
		Component: "worker",
		Family:    "lock_lease",
		Profile:   "lock_cache",
		Available: true,
		Mode:      cacheobservability.FamilyModeNamedProfile,
	})

	server := NewMetricsServerWithGovernance("127.0.0.1", 19091, "worker", registry)
	req := httptest.NewRequest(http.MethodGet, "/governance/redis", nil)
	rec := httptest.NewRecorder()
	server.server.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var payload struct {
		Component string `json:"component"`
		Summary   struct {
			Ready bool `json:"ready"`
		} `json:"summary"`
		Families []struct {
			Family string `json:"family"`
		} `json:"families"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload.Component != "worker" {
		t.Fatalf("component = %q, want worker", payload.Component)
	}
	if !payload.Summary.Ready {
		t.Fatal("summary.ready = false, want true")
	}
	if len(payload.Families) != 1 || payload.Families[0].Family != "lock_lease" {
		t.Fatalf("unexpected families payload: %+v", payload.Families)
	}
}
