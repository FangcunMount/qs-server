package rest

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/gin-gonic/gin"
)

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
