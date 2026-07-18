package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequestIDPropagatesIncomingHeaderToStandardContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID())
	router.GET("/", func(c *gin.Context) {
		if got := GetRequestIDFromContext(c); got != "request-existing" {
			t.Fatalf("Gin request ID = %q", got)
		}
		if got := RequestIDFromStandardContext(c.Request.Context()); got != "request-existing" {
			t.Fatalf("standard context request ID = %q", got)
		}
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(XRequestIDKey, "request-existing")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	if got := recorder.Header().Get(XRequestIDKey); got != "request-existing" {
		t.Fatalf("response request ID = %q", got)
	}
}
