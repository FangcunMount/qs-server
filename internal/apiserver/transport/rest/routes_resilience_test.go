package rest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/resilienceplane"
	"github.com/gin-gonic/gin"
)

func TestResilienceStatusRouteReturnsReadOnlySnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouter(Deps{
		Backpressure: []resilienceplane.BackpressureSnapshot{
			{
				Component:     "apiserver",
				Name:          "mysql",
				Dependency:    "mysql",
				Strategy:      "semaphore",
				Enabled:       true,
				MaxInflight:   16,
				TimeoutMillis: 200,
			},
		},
	})
	engine := gin.New()
	engine.Use(orgAdminSnapshotMiddleware())
	group := engine.Group("/internal/v1")
	router.registerResilienceInternalRoutes(group)

	req := httptest.NewRequest(http.MethodGet, "/internal/v1/resilience/status", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var payload struct {
		Component    string `json:"component"`
		Backpressure []struct {
			Name        string `json:"name"`
			MaxInflight int    `json:"max_inflight"`
		} `json:"backpressure"`
		Locks []struct {
			Name string `json:"name"`
		} `json:"locks"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload.Component != "apiserver" {
		t.Fatalf("component = %q, want apiserver", payload.Component)
	}
	if len(payload.Backpressure) != 1 || payload.Backpressure[0].Name != "mysql" || payload.Backpressure[0].MaxInflight != 16 {
		t.Fatalf("backpressure = %+v, want mysql max 16", payload.Backpressure)
	}
	if len(payload.Locks) != 3 {
		t.Fatalf("locks = %+v, want three scheduler locks", payload.Locks)
	}
}

func TestResilienceStatusHasNoMutationRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouter(Deps{})
	engine := gin.New()
	group := engine.Group("/internal/v1")
	router.registerResilienceInternalRoutes(group)

	req := httptest.NewRequest(http.MethodPost, "/internal/v1/resilience/status", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("POST /internal/v1/resilience/status status = %d, want 404", rec.Code)
	}
}
