package rest

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestProtectedGroupFailsClosedWhenIAMRuntimeIsUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	router := NewRouter(Deps{})
	group := engine.Group("/api/v1")
	router.applyProtectedGroupMiddlewares(group, "/api/v1")
	group.GET("/proof", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/proof", nil))

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
}
