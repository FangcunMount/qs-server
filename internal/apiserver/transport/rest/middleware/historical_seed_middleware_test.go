package middleware

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/historicalseed"
	"github.com/gin-gonic/gin"
)

type readCountingBody struct{ reads int }

func (b *readCountingBody) Read([]byte) (int, error) {
	b.reads++
	return 0, io.EOF
}

func (*readCountingBody) Close() error { return nil }

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

	body := &readCountingBody{}
	normalRequest := httptest.NewRequest(http.MethodPost, "/submit", nil)
	normalRequest.Body = body
	normal := httptest.NewRecorder()
	router.ServeHTTP(normal, normalRequest)
	if normal.Code != http.StatusNoContent {
		t.Fatalf("ordinary request changed when feature disabled: %d", normal.Code)
	}
	if body.reads != 0 {
		t.Fatalf("ordinary request body was read %d times", body.reads)
	}

	historical := httptest.NewRequest(http.MethodPost, "/submit", nil)
	historical.Header.Set(historicalseed.HeaderContext, "context")
	rejected := httptest.NewRecorder()
	router.ServeHTTP(rejected, historical)
	if rejected.Code != http.StatusForbidden {
		t.Fatalf("expected disabled historical request rejection, got %d", rejected.Code)
	}
}

func TestHistoricalSeedMiddlewareRejectsIncompleteHeadersWithoutReadingBody(t *testing.T) {
	router := gin.New()
	router.Use(HistoricalSeedMiddleware(&historicalseed.Verifier{Enabled: true, Secret: []byte("secret")}))
	router.POST("/submit", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	body := &readCountingBody{}
	request := httptest.NewRequest(http.MethodPost, "/submit", nil)
	request.Body = body
	request.Header.Set(historicalseed.HeaderContext, "context")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("expected incomplete header rejection, got %d", response.Code)
	}
	if body.reads != 0 {
		t.Fatalf("incomplete historical request body was read %d times", body.reads)
	}
}

func TestHistoricalSeedMiddlewareRejectsOversizedSignedBody(t *testing.T) {
	router := gin.New()
	router.Use(HistoricalSeedMiddleware(&historicalseed.Verifier{Enabled: true, Secret: []byte("secret")}))
	router.POST("/submit", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	request := httptest.NewRequest(http.MethodPost, "/submit", bytes.NewReader(make([]byte, historicalseed.MaxSignedBodyBytes+1)))
	request.Header.Set(historicalseed.HeaderContext, "context")
	request.Header.Set(historicalseed.HeaderRequestedAt, time.Now().Format(time.RFC3339Nano))
	request.Header.Set(historicalseed.HeaderSignature, "signature")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected oversized historical body rejection, got %d", response.Code)
	}
}
