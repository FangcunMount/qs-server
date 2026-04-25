package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestParseStatisticsSyncDateRange(t *testing.T) {
	gin.SetMode(gin.TestMode)

	req := httptest.NewRequest("POST", "/internal/v1/statistics/sync/daily?start_date=2026-04-01&end_date=2026-04-03", nil)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req

	opts, err := parseStatisticsSyncDateRange(c)
	if err != nil {
		t.Fatalf("parseStatisticsSyncDateRange returned error: %v", err)
	}
	if opts.StartDate == nil || opts.EndDate == nil {
		t.Fatalf("expected parsed date range")
	}
	if got := opts.StartDate.Format("2006-01-02"); got != "2026-04-01" {
		t.Fatalf("unexpected start date: %s", got)
	}
	if got := opts.EndDate.Format("2006-01-02"); got != "2026-04-04" {
		t.Fatalf("unexpected end date: %s", got)
	}
}

func TestParseStatisticsSyncDateRangeRejectsPartialRange(t *testing.T) {
	gin.SetMode(gin.TestMode)

	req := httptest.NewRequest("POST", "/internal/v1/statistics/sync/daily?start_date=2026-04-01", nil)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req

	if _, err := parseStatisticsSyncDateRange(c); err == nil {
		t.Fatalf("expected error for partial date range")
	}
}
