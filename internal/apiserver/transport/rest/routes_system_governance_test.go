package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	systemgov "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	"github.com/gin-gonic/gin"
)

type stubSystemGovernanceFacade struct {
	overview *systemgov.OverviewResponse
}

func (s stubSystemGovernanceFacade) GetOverview(context.Context, string) (*systemgov.OverviewResponse, error) {
	return s.overview, nil
}

func (s stubSystemGovernanceFacade) GetEvents(context.Context, string) (*systemgov.EventsView, error) {
	return &systemgov.EventsView{}, nil
}

func (s stubSystemGovernanceFacade) GetCache(context.Context, string) (*systemgov.CacheView, error) {
	return &systemgov.CacheView{}, nil
}

func (s stubSystemGovernanceFacade) GetResilience(context.Context, string) (*systemgov.ResilienceView, error) {
	return &systemgov.ResilienceView{}, nil
}

func (s stubSystemGovernanceFacade) ListActions(context.Context) (*systemgov.ActionsView, error) {
	return &systemgov.ActionsView{}, nil
}

func (s stubSystemGovernanceFacade) RunAction(context.Context, int64, string, systemgov.ActionRunRequest) (*systemgov.ActionRunResult, error) {
	return nil, nil
}

func TestSystemGovernanceOverviewRouteReturnsSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouter(Deps{
		SystemGovernanceFacade: stubSystemGovernanceFacade{overview: &systemgov.OverviewResponse{
			Window:          "5m",
			OverallSeverity: systemgov.SeverityHealthy,
			Metrics:         systemgov.MetricsSummary{Available: false, Reason: "prometheus not configured"},
		}},
	})
	engine := gin.New()
	engine.Use(orgAdminSnapshotMiddleware())
	group := engine.Group("/internal/v1")
	router.registerSystemGovernanceInternalRoutes(group)

	req := httptest.NewRequest(http.MethodGet, "/internal/v1/system-governance/overview?window=5m", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var payload struct {
		Data struct {
			Window          string `json:"window"`
			OverallSeverity string `json:"overall_severity"`
			Metrics         struct {
				Available bool `json:"available"`
			} `json:"metrics"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload.Data.Window != "5m" {
		t.Fatalf("window = %q, want 5m", payload.Data.Window)
	}
	if payload.Data.OverallSeverity != "healthy" {
		t.Fatalf("overall_severity = %q, want healthy", payload.Data.OverallSeverity)
	}
	if payload.Data.Metrics.Available {
		t.Fatal("metrics.available = true, want false when prometheus is unavailable")
	}
}
