package rest

import (
	"context"
	"strings"
	"testing"
	"time"

	statistics "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	"github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/gin-gonic/gin"
)

type statisticsRunStoreStub struct{}

func (statisticsRunStoreStub) Create(context.Context, statistics.Run) (*statistics.Run, error) {
	return nil, nil
}
func (statisticsRunStoreStub) UpdateProgress(context.Context, uint64, string, map[string]int64, map[string]int64, map[string]int64) error {
	return nil
}
func (statisticsRunStoreStub) AssertPublishable(context.Context, int64, time.Time) error {
	return nil
}
func (statisticsRunStoreStub) MarkDataCommitted(context.Context, uint64, time.Time) error {
	return nil
}
func (statisticsRunStoreStub) MarkCachePublished(context.Context, uint64, int64, time.Time) error {
	return nil
}
func (statisticsRunStoreStub) MarkCachePublishFailed(context.Context, uint64, int64, string, time.Time) error {
	return nil
}
func (statisticsRunStoreStub) RecordCacheResume(context.Context, uint64, uint64, string, string, int64, time.Time) error {
	return nil
}
func (statisticsRunStoreStub) MarkSucceeded(context.Context, uint64, time.Time) error { return nil }
func (statisticsRunStoreStub) MarkFailed(context.Context, uint64, string, string, string, time.Time) error {
	return nil
}
func (statisticsRunStoreStub) Get(context.Context, uint64) (*statistics.Run, error) { return nil, nil }
func (statisticsRunStoreStub) List(context.Context, int64, int) ([]statistics.Run, error) {
	return nil, nil
}

func TestRegisterStatisticsRoutesIsV2Only(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	rateLimit := options.NewRateLimitOptions()
	rateLimit.Enabled = false
	router := NewRouter(Deps{
		RateLimit: rateLimit,
		Statistics: StatisticsDeps{
			Enabled:     true,
			ReadService: statistics.NewReadService(nil),
			Coordinator: new(statistics.Coordinator),
			RunStore:    statisticsRunStoreStub{},
		},
	})
	protectedRouteRegistrar{router: router}.register(engine)
	internalRouteRegistrar{router: router}.register(engine)

	want := map[string]bool{
		"GET /api/v2/statistics/overview":                    false,
		"POST /api/v2/statistics/contents/batch":             false,
		"POST /internal/v2/statistics/runs":                  false,
		"POST /internal/v2/statistics/runs/:id/resume-cache": false,
	}
	for _, route := range engine.Routes() {
		key := route.Method + " " + route.Path
		if _, ok := want[key]; ok {
			want[key] = true
		}
		if strings.HasPrefix(route.Path, "/api/v1/statistics") || strings.HasPrefix(route.Path, "/internal/v1/statistics") {
			t.Fatalf("legacy Statistics route is registered: %s", key)
		}
	}
	for route, found := range want {
		if !found {
			t.Fatalf("route %s not registered", route)
		}
	}
}
