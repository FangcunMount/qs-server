package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/testeeaccess"
	pkgmiddleware "github.com/FangcunMount/qs-server/internal/pkg/middleware"
	"github.com/gin-gonic/gin"
)

type accessAuthorizerStub struct{ err error }

func (s accessAuthorizerStub) Authorize(context.Context, string, uint64) error {
	return s.err
}

func TestTesteeAccessMiddlewareRejectsUnlinkedUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("user_claims", &pkgmiddleware.UserClaims{UserID: "9"}); c.Next() })
	r.GET("/reports", TesteeAccessMiddleware(accessAuthorizerStub{err: testeeaccess.ErrAccessDenied}, "testee_id"), func(c *gin.Context) { c.Status(http.StatusOK) })
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/reports?testee_id=7", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "{\"error\":\"assessment access denied\"}" {
		t.Fatalf("body=%s", rec.Body.String())
	}
}

func TestTesteeAccessMiddlewareMapsUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("user_claims", &pkgmiddleware.UserClaims{UserID: "9"}); c.Next() })
	r.GET("/reports", TesteeAccessMiddleware(accessAuthorizerStub{err: fmt.Errorf("wrapped: %w", testeeaccess.ErrAccessUnavailable)}, "testee_id"), func(c *gin.Context) { c.Status(http.StatusOK) })
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/reports?testee_id=7", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "{\"error\":\"authorization temporarily unavailable\"}" {
		t.Fatalf("body=%s", rec.Body.String())
	}
}

func TestTesteeAccessMiddlewareAllowsLinkedUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("user_claims", &pkgmiddleware.UserClaims{UserID: "9"}); c.Next() })
	r.GET("/reports", TesteeAccessMiddleware(accessAuthorizerStub{}, "testee_id"), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/reports?testee_id=7", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
