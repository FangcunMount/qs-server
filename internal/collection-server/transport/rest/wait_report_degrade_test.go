package rest

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestWriteDegradedWaitReportSetsRetryAfterAndInterpretedPending(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/wait-report", nil)

	WriteDegradedWaitReport(c, 5)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if got := rec.Header().Get("Retry-After"); got != "5" {
		t.Fatalf("expected Retry-After 5, got %q", got)
	}
	body := rec.Body.String()
	if body == "" {
		t.Fatal("expected response body")
	}
	if !strings.Contains(body, `"status":"processing"`) || !strings.Contains(body, `"next_poll_after_ms":5000`) {
		t.Fatalf("unexpected body: %s", body)
	}
}
