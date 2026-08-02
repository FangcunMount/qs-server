package process

import (
	"net/http"
	"net/http/httptest"
	"testing"

	collectionconfig "github.com/FangcunMount/qs-server/internal/collection-server/config"
	collectionoptions "github.com/FangcunMount/qs-server/internal/collection-server/options"
	"github.com/FangcunMount/qs-server/internal/pkg/middleware"
	"github.com/gin-gonic/gin"
)

func TestBuiltCollectionServerRejectsRetiredHistoricalSeedBeforeCommonAndBusinessRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	opts := collectionoptions.NewOptions()
	opts.GenericServerRunOptions.Metrics = false
	opts.GenericServerRunOptions.Profiling = false
	cfg, err := collectionconfig.CreateConfigFromOptions(opts)
	if err != nil {
		t.Fatal(err)
	}
	server, err := buildGenericServer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	server.GET("/api/v1/proof", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	for _, path := range []string{"/healthz", "/api/v1/proof", "/missing"} {
		response := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header[http.CanonicalHeaderKey(middleware.HistoricalContextHeader)] = []string{""}
		server.ServeHTTP(response, req)
		if response.Code != http.StatusForbidden {
			t.Fatalf("GET %s status = %d, want %d", path, response.Code, http.StatusForbidden)
		}
	}
}
