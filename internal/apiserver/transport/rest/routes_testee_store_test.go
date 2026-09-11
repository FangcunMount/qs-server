package rest

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testeestore"
	"github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/gin-gonic/gin"
)

func TestTesteeStoreRoutesFailClosedWithoutIdentityDependencies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rate := options.NewRateLimitOptions()
	rate.Enabled = false
	engine := gin.New()
	NewRouter(Deps{RateLimit: rate, Actor: ActorDeps{TesteeStoreService: app.NewService(nil, nil)}}).RegisterRoutes(engine)
	for _, tc := range []struct{ method, path string }{{http.MethodPut, "/api/v1/testees/10/store"}, {http.MethodPost, "/api/v1/testees/10/store-transfers"}, {http.MethodGet, "/api/v1/testees/10/store-history"}} {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{"store_id":"2","expected_version":1,"reason":"test","request_id":"1"}`))
		req.Header.Set("Content-Type", "application/json")
		engine.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s %s=%d, want 503", tc.method, tc.path, recorder.Code)
		}
	}
}
