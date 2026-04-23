package apiserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/cachebootstrap"
	"github.com/FangcunMount/qs-server/internal/apiserver/container"
	resttransport "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheobservability"
	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/gin-gonic/gin"
)

func newGovernanceTestContainer(statuses ...cacheobservability.FamilyStatus) *container.Container {
	subsystem := cachebootstrap.NewSubsystem(
		"apiserver",
		nil,
		&genericoptions.RedisRuntimeOptions{},
		cachebootstrap.CacheOptions{},
	)
	subsystem.BindGovernance(cachebootstrap.GovernanceBindings{})
	for _, status := range statuses {
		subsystem.StatusRegistry().Update(status)
	}
	return container.NewContainerWithOptions(nil, nil, nil, container.ContainerOptions{
		CacheSubsystem: subsystem,
	})
}

func TestRouterReadyzReturnsServiceUnavailableWhenRuntimeNotReady(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	router := resttransport.NewRouter(newGovernanceTestContainer(
		cacheobservability.FamilyStatus{Component: "apiserver", Family: "query_result", Available: true},
		cacheobservability.FamilyStatus{Component: "apiserver", Family: "static_meta", Available: false, Degraded: true},
	).BuildRESTDeps(nil))
	router.RegisterRoutes(engine)

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
	router := resttransport.NewRouter(newGovernanceTestContainer(
		cacheobservability.FamilyStatus{Component: "apiserver", Family: "query_result", Profile: "query_cache", Available: true},
	).BuildRESTDeps(nil))
	router.RegisterRoutes(engine)

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
	router := resttransport.NewRouter((&container.Container{}).BuildRESTDeps(nil))
	router.RegisterRoutes(engine)

	for _, path := range []string{"/readyz", "/governance/redis"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want %d", path, rec.Code, http.StatusOK)
		}
	}
}
