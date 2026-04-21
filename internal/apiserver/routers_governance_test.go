package apiserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	cachegov "github.com/FangcunMount/qs-server/internal/apiserver/application/cachegovernance"
	"github.com/FangcunMount/qs-server/internal/apiserver/container"
	cacheinfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/cache"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheobservability"
	"github.com/gin-gonic/gin"
)

type fakeGovernanceStatusService struct {
	runtime *cacheobservability.RuntimeSnapshot
	status  *cachegov.StatusSnapshot
}

func (f fakeGovernanceStatusService) GetRuntime(context.Context) (*cacheobservability.RuntimeSnapshot, error) {
	if f.runtime == nil {
		return &cacheobservability.RuntimeSnapshot{
			GeneratedAt: time.Now(),
			Component:   "apiserver",
			Families:    []cacheobservability.FamilyStatus{},
			Summary:     cacheobservability.RuntimeSummary{Ready: true},
		}, nil
	}
	return f.runtime, nil
}

func (f fakeGovernanceStatusService) GetStatus(context.Context) (*cachegov.StatusSnapshot, error) {
	if f.status == nil {
		return &cachegov.StatusSnapshot{}, nil
	}
	return f.status, nil
}

func (f fakeGovernanceStatusService) GetHotset(context.Context, cacheinfra.WarmupKind, int64) (*cachegov.HotsetSnapshot, error) {
	return &cachegov.HotsetSnapshot{}, nil
}

func TestRouterReadyzReturnsServiceUnavailableWhenRuntimeNotReady(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	router := NewRouter(&container.Container{
		CacheGovernanceStatusService: fakeGovernanceStatusService{
			runtime: &cacheobservability.RuntimeSnapshot{
				GeneratedAt: time.Now(),
				Component:   "apiserver",
				Summary: cacheobservability.RuntimeSummary{
					FamilyTotal:      2,
					AvailableCount:   1,
					DegradedCount:    1,
					UnavailableCount: 1,
					Ready:            false,
				},
				Families: []cacheobservability.FamilyStatus{
					{Component: "apiserver", Family: "static_meta", Available: false, Degraded: true},
				},
			},
		},
	}, nil)
	router.registerPublicRoutes(engine)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}

	var payload struct {
		Status string `json:"status"`
		Redis  struct {
			Component string `json:"component"`
			Summary   struct {
				Ready bool `json:"ready"`
			} `json:"summary"`
		} `json:"redis"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload.Status != "degraded" {
		t.Fatalf("status payload = %q, want degraded", payload.Status)
	}
	if payload.Redis.Component != "apiserver" {
		t.Fatalf("redis.component = %q, want apiserver", payload.Redis.Component)
	}
	if payload.Redis.Summary.Ready {
		t.Fatal("redis.summary.ready = true, want false")
	}
}

func TestRouterGovernanceEndpointReturnsRuntimeSnapshotOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	router := NewRouter(&container.Container{
		CacheGovernanceStatusService: fakeGovernanceStatusService{
			runtime: &cacheobservability.RuntimeSnapshot{
				GeneratedAt: time.Now(),
				Component:   "apiserver",
				Summary: cacheobservability.RuntimeSummary{
					FamilyTotal:    1,
					AvailableCount: 1,
					Ready:          true,
				},
				Families: []cacheobservability.FamilyStatus{
					{Component: "apiserver", Family: "query_result", Profile: "query_cache", Available: true},
				},
			},
		},
	}, nil)
	router.registerPublicRoutes(engine)

	req := httptest.NewRequest(http.MethodGet, "/governance/redis", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var payload map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if _, ok := payload["warmup"]; ok {
		t.Fatal("warmup field present on /governance/redis, want absent")
	}
	var component string
	if err := json.Unmarshal(payload["component"], &component); err != nil {
		t.Fatalf("unmarshal component: %v", err)
	}
	if component != "apiserver" {
		t.Fatalf("component = %q, want apiserver", component)
	}
}

func TestRouterGovernanceEndpointsRemainPublicWhenGovernanceServiceUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	router := NewRouter(&container.Container{}, nil)
	router.registerPublicRoutes(engine)

	for _, path := range []string{"/readyz", "/governance/redis"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want %d", path, rec.Code, http.StatusOK)
		}
	}
}
