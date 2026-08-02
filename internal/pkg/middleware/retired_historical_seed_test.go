package middleware

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type unreadBody struct{ reads int }

func (b *unreadBody) Read([]byte) (int, error) {
	b.reads++
	return 0, io.EOF
}

func (*unreadBody) Close() error { return nil }

func TestRejectRetiredHistoricalSeedHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, header := range retiredHistoricalSeedHeaders {
		t.Run(header, func(t *testing.T) {
			body := &unreadBody{}
			request := httptest.NewRequest(http.MethodPost, "/submit", nil)
			request.Body = body
			request.Header[http.CanonicalHeaderKey(header)] = []string{""}

			response := httptest.NewRecorder()
			testHistoricalSeedGuardRouter().ServeHTTP(response, request)

			if response.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
			}
			if body.reads != 0 {
				t.Fatalf("request body was read %d times", body.reads)
			}
			var got map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if got["code"] != float64(http.StatusForbidden) || got["message"] != "historical context rejected" || got["error"] != "historical seed context is disabled" {
				t.Fatalf("unexpected response: %#v", got)
			}
		})
	}
}

func TestRejectRetiredHistoricalSeedHeadersPassesOrdinaryRequestWithoutReadingBody(t *testing.T) {
	body := &unreadBody{}
	request := httptest.NewRequest(http.MethodPost, "/submit", nil)
	request.Body = body
	response := httptest.NewRecorder()

	testHistoricalSeedGuardRouter().ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
	}
	if body.reads != 0 {
		t.Fatalf("request body was read %d times", body.reads)
	}
}

func testHistoricalSeedGuardRouter() http.Handler {
	router := gin.New()
	router.Use(RejectRetiredHistoricalSeedHeaders())
	router.POST("/submit", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	return router
}
