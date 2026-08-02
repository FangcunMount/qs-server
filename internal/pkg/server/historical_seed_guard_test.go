package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/middleware"
	"github.com/gin-gonic/gin"
)

type guardUnreadBody struct{ reads int }

func (b *guardUnreadBody) Read([]byte) (int, error) {
	b.reads++
	return 0, io.EOF
}

func (*guardUnreadBody) Close() error { return nil }

func TestGenericAPIServerRejectsRetiredHistoricalSeedHeadersBeforeRouting(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := NewConfig()
	cfg.EnableMetrics = false
	cfg.EnableProfiling = false
	server, err := cfg.Complete().New()
	if err != nil {
		t.Fatal(err)
	}
	server.POST("/business", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	headers := []string{
		middleware.HistoricalContextHeader,
		middleware.HistoricalRequestedAtHeader,
		middleware.HistoricalSignatureHeader,
	}
	paths := []string{"/healthz", "/version", "/business", "/missing"}
	for _, header := range headers {
		for _, path := range paths {
			t.Run(header+path, func(t *testing.T) {
				body := &guardUnreadBody{}
				req := httptest.NewRequest(http.MethodPost, path, nil)
				req.Body = body
				req.Header[http.CanonicalHeaderKey(header)] = []string{""}
				response := httptest.NewRecorder()

				server.ServeHTTP(response, req)

				if response.Code != http.StatusForbidden {
					t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
				}
				if body.reads != 0 {
					t.Fatalf("request body was read %d times", body.reads)
				}
			})
		}
	}
}

func TestGenericAPIServerPassesOrdinaryRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := NewConfig()
	cfg.EnableMetrics = false
	cfg.EnableProfiling = false
	server, err := cfg.Complete().New()
	if err != nil {
		t.Fatal(err)
	}
	server.POST("/business", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	body := &guardUnreadBody{}
	req := httptest.NewRequest(http.MethodPost, "/business", nil)
	req.Body = body
	response := httptest.NewRecorder()

	server.ServeHTTP(response, req)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
	}
	if body.reads != 0 {
		t.Fatalf("request body was read %d times", body.reads)
	}
}
