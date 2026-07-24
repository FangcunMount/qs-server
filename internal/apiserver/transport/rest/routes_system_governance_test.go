package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	systemgov "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	restmiddleware "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/middleware"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience"
	"github.com/gin-gonic/gin"
)

type stubSystemGovernanceFacade struct {
	overview    *systemgov.OverviewResponse
	events      *systemgov.EventsView
	cache       *systemgov.CacheView
	resilience  *systemgov.ResilienceView
	checkpoints *systemgov.CheckpointView
	candidates  *systemgov.RetryCandidatePage
	candidateFn func(int64, string, int) (*systemgov.RetryCandidatePage, error)
}

func (s stubSystemGovernanceFacade) GetOverview(context.Context, string) (*systemgov.OverviewResponse, error) {
	return s.overview, nil
}

func (s stubSystemGovernanceFacade) GetEvents(context.Context, int64, string) (*systemgov.EventsView, error) {
	if s.events != nil {
		return s.events, nil
	}
	return &systemgov.EventsView{}, nil
}

func (s stubSystemGovernanceFacade) ListRetryCandidates(_ context.Context, orgID int64, cursor string, limit int) (*systemgov.RetryCandidatePage, error) {
	if s.candidateFn != nil {
		return s.candidateFn(orgID, cursor, limit)
	}
	if s.candidates != nil {
		return s.candidates, nil
	}
	return &systemgov.RetryCandidatePage{}, nil
}

func TestSystemGovernanceRetryCandidatesAreOrgScopedAndBounded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	called := false
	router := newRouterWithBudgets(Deps{SystemGovernanceFacade: stubSystemGovernanceFacade{candidateFn: func(orgID int64, cursor string, limit int) (*systemgov.RetryCandidatePage, error) {
		called = true
		if orgID < 1 {
			t.Fatalf("orgID = %d, want resolved organization", orgID)
		}
		if cursor != "next-page" || limit != 25 {
			t.Fatalf("cursor/limit = %q/%d, want next-page/25", cursor, limit)
		}
		return &systemgov.RetryCandidatePage{Items: []systemgov.RetryCandidate{{Kind: "evaluation", ResourceID: "42", Disposition: "manual_required", Attempt: 3}}}, nil
	}}})
	engine := gin.New()
	engine.Use(orgAdminSnapshotMiddleware())
	engine.Use(func(c *gin.Context) {
		c.Set(restmiddleware.OrgIDKey, uint64(88))
		c.Next()
	})
	group := engine.Group("/internal/v1")
	router.registerSystemGovernanceInternalRoutes(group)

	req := httptest.NewRequest(http.MethodGet, "/internal/v1/system-governance/events/retry-candidates?cursor=next-page&limit=25", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !called {
		t.Fatalf("status/called = %d/%v, want 200/true", rec.Code, called)
	}

	badReq := httptest.NewRequest(http.MethodGet, "/internal/v1/system-governance/events/retry-candidates?limit=101", nil)
	badRec := httptest.NewRecorder()
	engine.ServeHTTP(badRec, badReq)
	if badRec.Code != http.StatusBadRequest {
		t.Fatalf("invalid limit status = %d, want 400", badRec.Code)
	}
}

func (s stubSystemGovernanceFacade) GetCache(context.Context, string) (*systemgov.CacheView, error) {
	if s.cache != nil {
		return s.cache, nil
	}
	return &systemgov.CacheView{}, nil
}

func (s stubSystemGovernanceFacade) GetResilience(context.Context, string) (*systemgov.ResilienceView, error) {
	if s.resilience != nil {
		return s.resilience, nil
	}
	return &systemgov.ResilienceView{}, nil
}

func (s stubSystemGovernanceFacade) GetCheckpoints(context.Context, string) (*systemgov.CheckpointView, error) {
	if s.checkpoints != nil {
		return s.checkpoints, nil
	}
	return &systemgov.CheckpointView{}, nil
}

func (s stubSystemGovernanceFacade) ListActions(context.Context) (*systemgov.ActionsView, error) {
	return &systemgov.ActionsView{}, nil
}

func (s stubSystemGovernanceFacade) RunAction(context.Context, int64, string, systemgov.ActionRunRequest) (*systemgov.ActionRunResult, error) {
	return nil, nil
}

func TestSystemGovernanceOverviewRouteReturnsSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newRouterWithBudgets(Deps{
		SystemGovernanceFacade: stubSystemGovernanceFacade{overview: &systemgov.OverviewResponse{
			Window:          "5m",
			OverallSeverity: systemgov.SeverityHealthy,
			Metrics:         systemgov.MetricsSummary{Available: false, Reason: "prometheus not configured"},
		}},
	})
	engine := gin.New()
	engine.Use(orgAdminSnapshotMiddleware())
	engine.Use(func(c *gin.Context) {
		c.Set(restmiddleware.OrgIDKey, uint64(88))
		c.Next()
	})
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

func TestSystemGovernanceEventsRouteReturnsAdditiveDrainFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newRouterWithBudgets(Deps{
		SystemGovernanceFacade: stubSystemGovernanceFacade{events: &systemgov.EventsView{
			Window: "5m",
			Summary: systemgov.EventDrainSummary{
				OutboxCount:         1,
				PendingCount:        12,
				StaleEventTypeCount: 1,
			},
			OutboxRows: []systemgov.EventOutboxRow{{
				Name:         "mysql",
				Store:        "mysql",
				PendingCount: 12,
				Severity:     systemgov.SeverityWarning,
			}},
			TypeRows: []systemgov.EventTypeRow{{
				Store:        "mysql",
				EventType:    "evaluation.requested",
				PendingCount: 9,
				Severity:     systemgov.SeverityWarning,
			}},
		}},
	})
	engine := gin.New()
	engine.Use(orgAdminSnapshotMiddleware())
	engine.Use(func(c *gin.Context) {
		c.Set(restmiddleware.OrgIDKey, uint64(88))
		c.Next()
	})
	group := engine.Group("/internal/v1")
	router.registerSystemGovernanceInternalRoutes(group)

	req := httptest.NewRequest(http.MethodGet, "/internal/v1/system-governance/events?window=5m", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var payload struct {
		Data struct {
			Summary struct {
				PendingCount       int `json:"pending_count"`
				StaleEventTypeRows int `json:"stale_event_type_count"`
			} `json:"summary"`
			OutboxRows []struct {
				Name         string `json:"name"`
				PendingCount int    `json:"pending_count"`
			} `json:"outbox_rows"`
			EventTypeRows []struct {
				EventType    string `json:"event_type"`
				PendingCount int    `json:"pending_count"`
			} `json:"event_type_rows"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload.Data.Summary.PendingCount != 12 || payload.Data.Summary.StaleEventTypeRows != 1 {
		t.Fatalf("summary = %+v, want pending=12 stale_event_type_count=1", payload.Data.Summary)
	}
	if len(payload.Data.OutboxRows) != 1 || payload.Data.OutboxRows[0].Name != "mysql" {
		t.Fatalf("outbox_rows = %+v, want mysql row", payload.Data.OutboxRows)
	}
	if len(payload.Data.EventTypeRows) != 1 || payload.Data.EventTypeRows[0].EventType != "evaluation.requested" {
		t.Fatalf("event_type_rows = %+v, want assessment.submitted row", payload.Data.EventTypeRows)
	}
}

func TestSystemGovernanceCacheRouteReturnsAdditiveWarmupFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newRouterWithBudgets(Deps{
		SystemGovernanceFacade: stubSystemGovernanceFacade{cache: &systemgov.CacheView{
			Window: "5m",
			Components: map[string]systemgov.ComponentCache{
				"collection-server": {
					Available: true, DiscoveredInstanceCount: 2, AvailableInstanceCount: 1, Partial: true,
					Instances: map[string]*observability.RuntimeSnapshot{
						"collection-a": {Component: "collection-server", InstanceID: "collection-a", Generation: "g1"},
					},
					TargetErrors: map[string]string{"10.0.0.2": "connection refused"},
				},
			},
			FamilyRows: []systemgov.CacheFamilyRow{{
				Component: "apiserver",
				Family:    "query_result",
				Severity:  systemgov.SeverityWarning,
			}},
			WarmupKinds: []systemgov.CacheWarmupKind{{
				Kind:                 "query.stats_overview",
				Family:               "query_result",
				ScopeExample:         "org:7:preset:30d",
				SupportsManualWarmup: true,
			}},
			Hotsets: []systemgov.CacheHotsetView{{
				Kind:      "query.stats_overview",
				Family:    "query_result",
				Available: true,
				Items: []systemgov.CacheHotsetItem{{
					Kind:  "query.stats_overview",
					Scope: "org:7:preset:30d",
					Score: 3,
				}},
			}},
		}},
	})
	engine := gin.New()
	engine.Use(orgAdminSnapshotMiddleware())
	group := engine.Group("/internal/v1")
	router.registerSystemGovernanceInternalRoutes(group)

	req := httptest.NewRequest(http.MethodGet, "/internal/v1/system-governance/cache?window=5m", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var payload struct {
		Data struct {
			Components map[string]struct {
				Available               bool                                     `json:"available"`
				Partial                 bool                                     `json:"partial"`
				DiscoveredInstanceCount int                                      `json:"discovered_instance_count"`
				AvailableInstanceCount  int                                      `json:"available_instance_count"`
				Instances               map[string]observability.RuntimeSnapshot `json:"instances"`
			} `json:"components"`
			FamilyRows []struct {
				Family string `json:"family"`
			} `json:"family_rows"`
			WarmupKinds []struct {
				Kind string `json:"kind"`
			} `json:"warmup_kinds"`
			Hotsets []struct {
				Items []struct {
					Scope string `json:"scope"`
				} `json:"items"`
			} `json:"hotsets"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	component := payload.Data.Components["collection-server"]
	if !component.Available || !component.Partial ||
		component.DiscoveredInstanceCount != 2 || component.AvailableInstanceCount != 1 ||
		component.Instances["collection-a"].Generation != "g1" {
		t.Fatalf("collection-server component = %+v, want partial instance snapshot", component)
	}
	if len(payload.Data.FamilyRows) != 1 || payload.Data.FamilyRows[0].Family != "query_result" {
		t.Fatalf("family_rows = %+v, want query_result row", payload.Data.FamilyRows)
	}
	if len(payload.Data.WarmupKinds) != 1 || payload.Data.WarmupKinds[0].Kind != "query.stats_overview" {
		t.Fatalf("warmup_kinds = %+v, want query.stats_overview", payload.Data.WarmupKinds)
	}
	if len(payload.Data.Hotsets) != 1 || len(payload.Data.Hotsets[0].Items) != 1 {
		t.Fatalf("hotsets = %+v, want one recommendation item", payload.Data.Hotsets)
	}
	if payload.Data.Hotsets[0].Items[0].Scope != "org:7:preset:30d" {
		t.Fatalf("hotset scope = %q, want org:7:preset:30d", payload.Data.Hotsets[0].Items[0].Scope)
	}
}

func TestSystemGovernanceResilienceRouteReturnsAdditivePressureFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newRouterWithBudgets(Deps{
		SystemGovernanceFacade: stubSystemGovernanceFacade{resilience: &systemgov.ResilienceView{
			Window: "5m",
			Summary: systemgov.ResilienceSummary{
				ComponentCount:      2,
				InstanceCount:       3,
				CriticalQueueCount:  1,
				BackpressureCount:   1,
				MaxQueueUtilization: 0.95,
			},
			Components: map[string]systemgov.ComponentResilience{
				"collection-server": {
					Available: true, DiscoveredInstanceCount: 2, AvailableInstanceCount: 2,
					Instances: map[string]*resilience.RuntimeSnapshot{
						"collection-a": {Component: "collection-server", InstanceID: "collection-a", Generation: "g1"},
						"collection-b": {Component: "collection-server", InstanceID: "collection-b", Generation: "g2"},
					},
				},
			},
			QueueRows: []systemgov.ResilienceQueueRow{{
				Component:   "collection-server",
				InstanceID:  "collection-a",
				Name:        "answersheet_submit",
				Depth:       95,
				Capacity:    100,
				Utilization: 0.95,
				Severity:    systemgov.SeverityCritical,
			}},
			BackpressureRows: []systemgov.ResilienceBackpressureRow{{
				Component:   "apiserver",
				Name:        "mysql",
				Dependency:  "mysql",
				InFlight:    8,
				MaxInflight: 10,
				Utilization: 0.8,
				Severity:    systemgov.SeverityWarning,
			}},
			CapabilityRows: []systemgov.ResilienceCapabilityRow{{
				Component:  "apiserver",
				Kind:       "rate_limit",
				Name:       "api_global",
				Configured: true,
				Degraded:   true,
				Severity:   systemgov.SeverityWarning,
			}},
		}},
	})
	engine := gin.New()
	engine.Use(orgAdminSnapshotMiddleware())
	group := engine.Group("/internal/v1")
	router.registerSystemGovernanceInternalRoutes(group)

	req := httptest.NewRequest(http.MethodGet, "/internal/v1/system-governance/resilience?window=5m", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var payload struct {
		Data struct {
			Summary struct {
				ComponentCount      int     `json:"component_count"`
				InstanceCount       int     `json:"instance_count"`
				CriticalQueueCount  int     `json:"critical_queue_count"`
				MaxQueueUtilization float64 `json:"max_queue_utilization"`
			} `json:"summary"`
			QueueRows []struct {
				Name        string  `json:"name"`
				InstanceID  string  `json:"instance_id"`
				Utilization float64 `json:"utilization"`
			} `json:"queue_rows"`
			Components map[string]struct {
				Instances map[string]resilience.RuntimeSnapshot `json:"instances"`
			} `json:"components"`
			BackpressureRows []struct {
				Name       string `json:"name"`
				Dependency string `json:"dependency"`
			} `json:"backpressure_rows"`
			CapabilityRows []struct {
				Kind string `json:"kind"`
				Name string `json:"name"`
			} `json:"capability_rows"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload.Data.Summary.ComponentCount != 2 || payload.Data.Summary.InstanceCount != 3 ||
		payload.Data.Summary.CriticalQueueCount != 1 || payload.Data.Summary.MaxQueueUtilization != 0.95 {
		t.Fatalf("summary = %+v, want additive resilience summary", payload.Data.Summary)
	}
	if len(payload.Data.QueueRows) != 1 || payload.Data.QueueRows[0].Name != "answersheet_submit" ||
		payload.Data.QueueRows[0].InstanceID != "collection-a" {
		t.Fatalf("queue_rows = %+v, want answersheet_submit row", payload.Data.QueueRows)
	}
	if len(payload.Data.Components["collection-server"].Instances) != 2 {
		t.Fatalf("components = %+v, want two collection instances", payload.Data.Components)
	}
	if len(payload.Data.BackpressureRows) != 1 || payload.Data.BackpressureRows[0].Dependency != "mysql" {
		t.Fatalf("backpressure_rows = %+v, want mysql row", payload.Data.BackpressureRows)
	}
	if len(payload.Data.CapabilityRows) != 1 || payload.Data.CapabilityRows[0].Kind != "rate_limit" {
		t.Fatalf("capability_rows = %+v, want rate_limit row", payload.Data.CapabilityRows)
	}
}
