package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	authzapp "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	restmiddleware "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/middleware"
	"github.com/gin-gonic/gin"
)

type fakeEventStatusService struct {
	snapshot *appEventing.StatusSnapshot
	err      error
}

func (s fakeEventStatusService) GetStatus(context.Context) (*appEventing.StatusSnapshot, error) {
	return s.snapshot, s.err
}

func TestEventStatusRouteReturnsReadOnlySnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	router := newRouterWithBudgets(Deps{
		EventStatusService: fakeEventStatusService{snapshot: &appEventing.StatusSnapshot{
			GeneratedAt: now,
			Catalog: appEventing.CatalogSummary{
				TopicCount:         4,
				EventCount:         19,
				DurableOutboxCount: 12,
			},
			Outboxes: []appEventing.OutboxSummary{
				{
					Name: "mysql",
					Buckets: []outboxport.StatusBucket{
						{Status: "pending", Count: 2},
					},
				},
			},
			Events: []appEventing.EventSummary{{
				Type: "answersheet.submitted", Owner: "survey/answersheet", Delivery: "durable_outbox",
				Profile: "mongo_domain_events", Immediate: true, Priority: "p0", Handler: "answersheet_submitted_handler",
				Idempotency: "answersheet-id-lease-and-ensure-assessment", Settlement: "handler_error_nack",
			}},
			Profiles: []appEventing.ProfileSummary{{
				Name: "mongo_domain_events", EventCount: 3, Running: true, RelayEnabled: true,
			}},
			Consumers: []appEventing.ConsumerSummary{{
				ID: "modelcatalog.hot_rank_projection", EventType: "answersheet.submitted", Runtime: "apiserver",
				Topic: "qs.evaluation.lifecycle", Channel: "qs-apiserver-modelcatalog-hot-rank-v1", Enabled: true, Healthy: true,
			}},
		}},
	})
	engine := gin.New()
	engine.Use(orgAdminSnapshotMiddleware())
	group := engine.Group("/internal/v1")
	router.registerEventStatusInternalRoutes(group)

	req := httptest.NewRequest(http.MethodGet, "/internal/v1/events/status", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var payload struct {
		Catalog struct {
			EventCount int `json:"event_count"`
		} `json:"catalog"`
		Outboxes []struct {
			Name    string `json:"name"`
			Buckets []struct {
				Status string `json:"status"`
				Count  int64  `json:"count"`
			} `json:"buckets"`
		} `json:"outboxes"`
		Events []struct {
			Type    string `json:"type"`
			Profile string `json:"profile"`
		} `json:"events"`
		Profiles []struct {
			Name       string `json:"name"`
			EventCount int    `json:"event_count"`
		} `json:"profiles"`
		Consumers []struct {
			ID      string `json:"id"`
			Channel string `json:"channel"`
		} `json:"consumers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload.Catalog.EventCount != 19 {
		t.Fatalf("event_count = %d, want 19", payload.Catalog.EventCount)
	}
	if len(payload.Outboxes) != 1 || payload.Outboxes[0].Buckets[0].Count != 2 {
		t.Fatalf("outboxes = %#v, want one pending bucket", payload.Outboxes)
	}
	if len(payload.Events) != 1 || payload.Events[0].Type != "answersheet.submitted" || payload.Events[0].Profile != "mongo_domain_events" {
		t.Fatalf("events = %#v, want effective event contract", payload.Events)
	}
	if len(payload.Profiles) != 1 || payload.Profiles[0].Name != "mongo_domain_events" || payload.Profiles[0].EventCount != 3 {
		t.Fatalf("profiles = %#v, want runtime profile status", payload.Profiles)
	}
	if len(payload.Consumers) != 1 || payload.Consumers[0].ID != "modelcatalog.hot_rank_projection" || payload.Consumers[0].Channel != "qs-apiserver-modelcatalog-hot-rank-v1" {
		t.Fatalf("consumers = %#v, want hot-rank status", payload.Consumers)
	}
}

func orgAdminSnapshotMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(restmiddleware.AuthzSnapshotKey, &authzapp.Snapshot{
			Permissions: []authzapp.Permission{
				{Resource: "qs:*", Action: ".*"},
			},
		})
		c.Next()
	}
}

func TestEventStatusHasNoRepairRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newRouterWithBudgets(Deps{
		EventStatusService: fakeEventStatusService{snapshot: &appEventing.StatusSnapshot{}},
	})
	engine := gin.New()
	group := engine.Group("/internal/v1")
	router.registerEventStatusInternalRoutes(group)

	req := httptest.NewRequest(http.MethodPost, "/internal/v1/events/status", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("POST /internal/v1/events/status status = %d, want 404", rec.Code)
	}
}
