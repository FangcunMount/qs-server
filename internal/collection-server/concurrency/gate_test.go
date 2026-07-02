package concurrency

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestGateTryAcquireAndRelease(t *testing.T) {
	gate := NewGate(1)
	if !gate.TryAcquire() {
		t.Fatal("expected first acquire to succeed")
	}
	if gate.TryAcquire() {
		t.Fatal("expected second acquire to fail")
	}
	gate.Release()
	if !gate.TryAcquire() {
		t.Fatal("expected acquire after release")
	}
}

func TestGateWaitMiddlewareRejectsWhenSlotsExhausted(t *testing.T) {
	gate := NewGate(1)
	if !gate.TryAcquire() {
		t.Fatal("expected first acquire to succeed")
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/scales", nil)

	rejected := false
	mw := gate.WaitMiddleware(10*time.Millisecond, func(c *gin.Context) {
		rejected = true
		WriteServiceUnavailableForTest(c)
	})
	mw(c)

	if !rejected {
		t.Fatal("expected onReject to run")
	}
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", recorder.Code)
	}
}

func WriteServiceUnavailableForTest(c *gin.Context) {
	c.JSON(http.StatusServiceUnavailable, gin.H{"code": http.StatusServiceUnavailable})
}

func TestNilGateAlwaysAcquires(t *testing.T) {
	var gate *Gate
	if !gate.TryAcquire() {
		t.Fatal("nil gate should allow acquire")
	}
	gate.Release()
}
