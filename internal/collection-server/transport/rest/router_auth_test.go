package rest

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestApplyIAMAuthFailsClosedWhenIAMIsUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api := engine.Group("/api/v1")
	(&Router{}).applyIAMAuth(api, func(c *gin.Context) bool {
		return c.Request.URL.Path == "/api/v1/public-catalog"
	})
	api.GET("/protected", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	api.GET("/public-catalog", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	protected := httptest.NewRecorder()
	engine.ServeHTTP(protected, httptest.NewRequest(http.MethodGet, "/api/v1/protected", nil))
	if protected.Code != http.StatusServiceUnavailable {
		t.Fatalf("protected status = %d, want %d", protected.Code, http.StatusServiceUnavailable)
	}

	public := httptest.NewRecorder()
	engine.ServeHTTP(public, httptest.NewRequest(http.MethodGet, "/api/v1/public-catalog", nil))
	if public.Code != http.StatusNoContent {
		t.Fatalf("public status = %d, want %d", public.Code, http.StatusNoContent)
	}
}
