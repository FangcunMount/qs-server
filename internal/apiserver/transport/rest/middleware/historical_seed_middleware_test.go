package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/historicalseed"
	"github.com/gin-gonic/gin"
)

func TestHistoricalSeedMiddlewareOptionalAndVerified(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	occurredAt := time.Date(2025, 1, 1, 1, 0, 0, 0, time.UTC)
	historical := historicalseed.Context{
		BatchID: "batch", ScenarioID: "scenario", OrgID: 1, Version: historicalseed.Version1,
		Timeline: historicalseed.Timeline{EntryResolvedAt: &occurredAt},
	}
	encoded, err := historicalseed.Encode(historical)
	if err != nil {
		t.Fatal(err)
	}
	verifier := &historicalseed.Verifier{
		Enabled: true, Secret: []byte("secret"), AllowedOrgIDs: map[uint64]struct{}{1: {}},
		Earliest: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), Latest: time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC),
		Location: time.UTC, MaxSkew: 5 * time.Minute, Now: func() time.Time { return now },
	}
	router := gin.New()
	router.Use(HistoricalSeedMiddleware(verifier))
	router.POST("/submit", func(c *gin.Context) {
		got, ok := historicalseed.FromContext(c.Request.Context())
		if !ok || got.BatchID != "batch" {
			c.Status(http.StatusInternalServerError)
			return
		}
		body, _ := c.GetRawData()
		if string(body) != `{"answer":"A"}` {
			c.Status(http.StatusBadRequest)
			return
		}
		c.Status(http.StatusNoContent)
	})

	body := []byte(`{"answer":"A"}`)
	req := httptest.NewRequest(http.MethodPost, "/submit?testee_id=7", bytes.NewReader(body))
	requestedAt := now.Format(time.RFC3339Nano)
	req.Header.Set(historicalseed.HeaderContext, encoded)
	req.Header.Set(historicalseed.HeaderRequestedAt, requestedAt)
	req.Header.Set(historicalseed.HeaderSignature, historicalseed.Sign(http.MethodPost, req.URL.RequestURI(), body, requestedAt, encoded, []byte("secret")))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusNoContent {
		t.Fatalf("unexpected status %d: %s", response.Code, response.Body.String())
	}
}

func TestHistoricalSeedMiddlewareRejectsHeadersWhenDisabled(t *testing.T) {
	router := gin.New()
	router.Use(HistoricalSeedMiddleware(&historicalseed.Verifier{}))
	router.POST("/submit", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	normal := httptest.NewRecorder()
	router.ServeHTTP(normal, httptest.NewRequest(http.MethodPost, "/submit", nil))
	if normal.Code != http.StatusNoContent {
		t.Fatalf("ordinary request changed when feature disabled: %d", normal.Code)
	}

	historical := httptest.NewRequest(http.MethodPost, "/submit", nil)
	historical.Header.Set(historicalseed.HeaderContext, "context")
	rejected := httptest.NewRecorder()
	router.ServeHTTP(rejected, historical)
	if rejected.Code != http.StatusForbidden {
		t.Fatalf("expected disabled historical request rejection, got %d", rejected.Code)
	}
}
