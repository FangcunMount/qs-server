package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	testeeapp "github.com/FangcunMount/qs-server/internal/collection-server/application/testee"
	pkgmiddleware "github.com/FangcunMount/qs-server/internal/pkg/middleware"
	"github.com/gin-gonic/gin"
)

type testeeResolverStub struct{ profile string }

func (s testeeResolverStub) GetTestee(context.Context, uint64) (*testeeapp.TesteeResponse, error) {
	return &testeeapp.TesteeResponse{IAMProfileID: s.profile}, nil
}

type linkCheckerStub struct{ allowed bool }

func (linkCheckerStub) IsEnabled() bool { return true }
func (s linkCheckerStub) HasActiveProfileLink(context.Context, string, string) (bool, error) {
	return s.allowed, nil
}

func TestTesteeProfileLinkMiddlewareRejectsUnlinkedUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("user_claims", &pkgmiddleware.UserClaims{UserID: "9"}); c.Next() })
	r.GET("/reports", TesteeProfileLinkMiddleware(testeeResolverStub{profile: "p-7"}, linkCheckerStub{}, "testee_id"), func(c *gin.Context) { c.Status(http.StatusOK) })
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/reports?testee_id=7", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestTesteeProfileLinkMiddlewareAllowsLinkedUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("user_claims", &pkgmiddleware.UserClaims{UserID: "9"}); c.Next() })
	r.GET("/reports", TesteeProfileLinkMiddleware(testeeResolverStub{profile: "p-7"}, linkCheckerStub{allowed: true}, "testee_id"), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/reports?testee_id=7", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
