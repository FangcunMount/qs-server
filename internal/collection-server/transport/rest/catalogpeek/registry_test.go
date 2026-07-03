package catalogpeek_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/personalitymodel"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/questionnaire"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/scale"
	"github.com/FangcunMount/qs-server/internal/collection-server/transport/rest/catalogpeek"
	"github.com/gin-gonic/gin"
)

func TestRegistryPeekRouteMatrix(t *testing.T) {
	gin.SetMode(gin.TestMode)

	scaleCache := scale.NewLocalCatalogCache(scale.LocalCatalogCacheOptions{TTL: time.Minute, MaxEntries: 16})
	scaleCache.SetDetail("ABC", &scale.ScaleResponse{Code: "ABC"})
	scaleSvc := scale.NewQueryService(nil, scaleCache, false)

	personalityCache := personalitymodel.NewLocalCatalogCache(personalitymodel.LocalCatalogCacheOptions{TTL: time.Minute, MaxEntries: 16})
	personalityCache.SetDetail("PM1", &personalitymodel.PersonalityModelResponse{Code: "PM1"})
	personalitySvc := personalitymodel.NewQueryService(nil, personalityCache, false)

	questionnaireCache := questionnaire.NewLocalCache(questionnaire.LocalCacheOptions{TTL: time.Minute, MaxEntries: 16})
	questionnaireCache.Set("Q1", "v1", &questionnaire.QuestionnaireResponse{Code: "Q1", Version: "v1"})
	questionnaireSvc := questionnaire.NewQueryService(nil, questionnaireCache, false)

	registry := catalogpeek.NewRegistry()
	catalogpeek.RegisterCatalogL1(registry, scaleSvc, personalitySvc, questionnaireSvc)

	peekViaRoute := func(method, path string) bool {
		var got bool
		engine := gin.New()
		engine.GET("/api/v1/scales/:code", func(c *gin.Context) { got = registry.Peek(c) })
		engine.GET("/api/v1/personality-models/:code", func(c *gin.Context) { got = registry.Peek(c) })
		engine.GET("/api/v1/questionnaires/:code", func(c *gin.Context) { got = registry.Peek(c) })
		engine.POST("/api/v1/scales/:code", func(c *gin.Context) { got = registry.Peek(c) })
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, httptest.NewRequest(method, path, nil))
		return got
	}

	cases := []struct {
		name   string
		method string
		path   string
		want   bool
	}{
		{name: "scale_detail_hit", method: http.MethodGet, path: "/api/v1/scales/ABC", want: true},
		{name: "scale_detail_miss", method: http.MethodGet, path: "/api/v1/scales/missing", want: false},
		{name: "personality_detail_hit", method: http.MethodGet, path: "/api/v1/personality-models/PM1", want: true},
		{name: "questionnaire_detail_hit", method: http.MethodGet, path: "/api/v1/questionnaires/Q1?version=v1", want: true},
		{name: "questionnaire_detail_version_miss", method: http.MethodGet, path: "/api/v1/questionnaires/Q1?version=other", want: false},
		{name: "non_get", method: http.MethodPost, path: "/api/v1/scales/ABC", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := peekViaRoute(tc.method, tc.path); got != tc.want {
				t.Fatalf("Peek() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRegistryPeekRejectsNonGET(t *testing.T) {
	gin.SetMode(gin.TestMode)
	registry := catalogpeek.NewRegistry()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/scales/ABC", nil)
	if registry.Peek(ctx) {
		t.Fatal("expected false for non-GET")
	}
}
