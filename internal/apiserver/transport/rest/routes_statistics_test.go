package rest

import (
	"context"
	"encoding/json"
	"os"
	"sort"
	"testing"
	"time"

	statisticsv2 "github.com/FangcunMount/qs-server/internal/apiserver/application/statisticsv2"
	"github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/gin-gonic/gin"
)

type statisticsV2RunStoreStub struct{}

func (statisticsV2RunStoreStub) Create(context.Context, statisticsv2.Run) (*statisticsv2.Run, error) {
	return nil, nil
}
func (statisticsV2RunStoreStub) UpdateProgress(context.Context, uint64, string, map[string]int64, map[string]int64, map[string]int64) error {
	return nil
}
func (statisticsV2RunStoreStub) AssertPublishable(context.Context, int64, time.Time) error {
	return nil
}
func (statisticsV2RunStoreStub) MarkDataCommitted(context.Context, uint64, time.Time) error {
	return nil
}
func (statisticsV2RunStoreStub) MarkCachePublishFailed(context.Context, uint64, string, time.Time) error {
	return nil
}
func (statisticsV2RunStoreStub) RecordCacheResume(context.Context, uint64, uint64, string, string, time.Time) error {
	return nil
}
func (statisticsV2RunStoreStub) MarkSucceeded(context.Context, uint64, time.Time) error { return nil }
func (statisticsV2RunStoreStub) MarkFailed(context.Context, uint64, string, string, string, time.Time) error {
	return nil
}
func (statisticsV2RunStoreStub) Get(context.Context, uint64) (*statisticsv2.Run, error) {
	return nil, nil
}
func (statisticsV2RunStoreStub) List(context.Context, int64, int) ([]statisticsv2.Run, error) {
	return nil, nil
}

func TestRegisterStatisticsRoutesExcludesLegacyBatchPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	rateLimit := options.NewRateLimitOptions()
	rateLimit.Enabled = false
	router := NewRouter(Deps{RateLimit: rateLimit, Statistics: StatisticsDeps{Enabled: true}})
	router.registerStatisticsProtectedRoutes(engine.Group("/api/v1"))

	wantCurrent := false
	for _, route := range engine.Routes() {
		if route.Method == "POST" && route.Path == "/api/v1/statistics/questionnaires/batch" {
			t.Fatal("legacy statistics questionnaire batch route is registered")
		}
		if route.Method == "POST" && route.Path == "/api/v1/statistics/contents/batch" {
			wantCurrent = true
		}
	}
	if !wantCurrent {
		t.Fatal("current statistics content batch route is not registered")
	}
}

func TestStatisticsV1PublicRoutesMatchGolden(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	rateLimit := options.NewRateLimitOptions()
	rateLimit.Enabled = false
	router := NewRouter(Deps{RateLimit: rateLimit, Statistics: StatisticsDeps{Enabled: true}})
	router.registerStatisticsProtectedRoutes(engine.Group("/api/v1"))

	var got []string
	for _, route := range engine.Routes() {
		got = append(got, route.Method+" "+route.Path)
	}
	sort.Strings(got)
	payload, err := os.ReadFile("testdata/statistics_v1_routes.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var want []string
	if err := json.Unmarshal(payload, &want); err != nil {
		t.Fatal(err)
	}
	if string(mustJSON(t, got)) != string(mustJSON(t, want)) {
		t.Fatalf("V1 statistics routes changed\ngot:  %v\nwant: %v", got, want)
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func TestRegisterStatisticsV2Routes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	rateLimit := options.NewRateLimitOptions()
	rateLimit.Enabled = false
	router := NewRouter(Deps{RateLimit: rateLimit, Statistics: StatisticsDeps{Enabled: true, V2ReadService: statisticsv2.NewReadService(nil), V2Coordinator: new(statisticsv2.Coordinator), V2RunStore: statisticsV2RunStoreStub{}}})
	router.registerStatisticsV2ProtectedRoutes(engine.Group("/api/v2"))
	router.registerStatisticsV2InternalRoutes(engine.Group("/internal/v2"))
	want := map[string]bool{"GET /api/v2/statistics/overview": false, "POST /api/v2/statistics/contents/batch": false, "POST /internal/v2/statistics/runs": false, "POST /internal/v2/statistics/runs/:id/resume-cache": false}
	for _, route := range engine.Routes() {
		key := route.Method + " " + route.Path
		if _, ok := want[key]; ok {
			want[key] = true
		}
	}
	for route, found := range want {
		if !found {
			t.Fatalf("route %s not registered", route)
		}
	}
}
