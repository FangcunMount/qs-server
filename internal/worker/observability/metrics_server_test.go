package observability

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/cachegovernance/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/resilienceplane"
)

func TestMetricsServerReadyzReturnsServiceUnavailableWhenFamilyDegraded(t *testing.T) {
	registry := observability.NewFamilyStatusRegistry("worker")
	registry.Update(observability.FamilyStatus{
		Component: "worker",
		Family:    "lock_lease",
		Profile:   "lock_cache",
		Available: false,
		Degraded:  true,
		Mode:      observability.FamilyModeDegraded,
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
	registry := observability.NewFamilyStatusRegistry("worker")
	registry.Update(observability.FamilyStatus{
		Component: "worker",
		Family:    "lock_lease",
		Profile:   "lock_cache",
		Available: true,
		Mode:      observability.FamilyModeNamedProfile,
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

func TestMetricsServerResilienceEndpointReturnsSnapshot(t *testing.T) {
	registry := observability.NewFamilyStatusRegistry("worker")
	server := NewMetricsServerWithGovernanceAndResilience(
		"127.0.0.1",
		19091,
		"worker",
		registry,
		func() resilienceplane.RuntimeSnapshot {
			snapshot := resilienceplane.NewRuntimeSnapshot("worker", time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC))
			snapshot.DuplicateSuppression = []resilienceplane.CapabilitySnapshot{
				{Name: "answersheet_submitted", Kind: resilienceplane.ProtectionDuplicateSuppression.String(), Strategy: "redis_lock", Configured: true},
			}
			return resilienceplane.FinalizeRuntimeSnapshot(snapshot)
		},
	)
	req := httptest.NewRequest(http.MethodGet, "/governance/resilience", nil)
	rec := httptest.NewRecorder()
	server.server.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var payload struct {
		Component string `json:"component"`
		Summary   struct {
			Ready           bool `json:"ready"`
			CapabilityCount int  `json:"capability_count"`
		} `json:"summary"`
		DuplicateSuppression []struct {
			Name string `json:"name"`
		} `json:"duplicate_suppression"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload.Component != "worker" {
		t.Fatalf("component = %q, want worker", payload.Component)
	}
	if !payload.Summary.Ready || payload.Summary.CapabilityCount != 1 {
		t.Fatalf("summary = %+v, want ready with one capability", payload.Summary)
	}
	if len(payload.DuplicateSuppression) != 1 || payload.DuplicateSuppression[0].Name != "answersheet_submitted" {
		t.Fatalf("duplicate_suppression = %+v, want answersheet_submitted", payload.DuplicateSuppression)
	}
}
