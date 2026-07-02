package rest

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/FangcunMount/qs-server/internal/collection-server/concurrency"
	"github.com/FangcunMount/qs-server/internal/collection-server/container"
	"github.com/FangcunMount/qs-server/internal/collection-server/options"
	"github.com/gin-gonic/gin"
)

func TestTryQueryConcurrencyHandlersRejectWhenSlotsFull(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	gate := concurrency.NewGate(1)
	if !gate.TryAcquire() {
		t.Fatal("expected to acquire sole query slot")
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/questionnaires", nil)

	handlers := tryQueryConcurrencyHandlers(gate, func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	handlers[0](c)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", recorder.Code)
	}
}

func TestRouterConcurrencyMaxWaitFromOptions(t *testing.T) {
	t.Parallel()

	opts := options.NewOptions()
	opts.Concurrency.MaxWaitMs = 2500
	router := &Router{container: container.NewContainer(opts, nil, nil, nil)}
	if got := router.concurrencyMaxWait().Milliseconds(); got != 2500 {
		t.Fatalf("concurrencyMaxWait = %dms, want 2500ms", got)
	}
}
