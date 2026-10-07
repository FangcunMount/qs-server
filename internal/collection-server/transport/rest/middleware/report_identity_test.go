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
	if rec.Body.String() != "{\"error\":\"testee access denied\"}" {
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

type recordingAccessAuthorizer struct {
	allowedID uint64
	calls     []uint64
}

func (s *recordingAccessAuthorizer) Authorize(_ context.Context, _ string, id uint64) error {
	s.calls = append(s.calls, id)
	if id != s.allowedID {
		return testeeaccess.ErrAccessDenied
	}
	return nil
}

func TestTesteeAccessMiddlewareAuthorizesPathResource(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, endpoint := range []struct{ name, method, route, path string }{
		{"get", http.MethodGet, "/testees/:id", "/testees/7"},
		{"put", http.MethodPut, "/testees/:id", "/testees/7"},
		{"care-context", http.MethodGet, "/testees/:id/care-context", "/testees/7/care-context"},
	} {
		for _, scenario := range []struct {
			name, query string
			allowedID   uint64
			status      int
		}{
			{"path-without-query", "", 7, http.StatusNoContent},
			{"query-cannot-substitute-authorized-object", "?id=9", 9, http.StatusForbidden},
			{"query-cannot-deny-authorized-path", "?id=9", 7, http.StatusNoContent},
			{"malformed-query-does-not-override-path", "?id=invalid", 7, http.StatusNoContent},
		} {
			t.Run(endpoint.name+"/"+scenario.name, func(t *testing.T) {
				authorizer := &recordingAccessAuthorizer{allowedID: scenario.allowedID}
				router := gin.New()
				router.Use(func(c *gin.Context) { c.Set("user_claims", &pkgmiddleware.UserClaims{UserID: "9"}); c.Next() })
				reached := false
				router.Handle(endpoint.method, endpoint.route, TesteeAccessMiddleware(authorizer, "id"), func(c *gin.Context) {
					reached = true
					if c.Param("id") != "7" {
						t.Fatalf("operation path id=%q", c.Param("id"))
					}
					if verified, _ := c.Get(TesteeIDKey); verified != uint64(7) {
						t.Fatalf("authorized id=%v, operation id=7", verified)
					}
					c.Status(http.StatusNoContent)
				})
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, httptest.NewRequest(endpoint.method, endpoint.path+scenario.query, nil))
				if rec.Code != scenario.status {
					t.Fatalf("status=%d want=%d body=%s", rec.Code, scenario.status, rec.Body.String())
				}
				if len(authorizer.calls) != 1 || authorizer.calls[0] != 7 {
					t.Fatalf("authorized resources=%v, want [7]", authorizer.calls)
				}
				if reached != (scenario.status == http.StatusNoContent) {
					t.Fatalf("operation reached=%v status=%d", reached, rec.Code)
				}
			})
		}
	}
}

func TestTesteeAccessMiddlewareRejectsInvalidPathWithoutQueryFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, pathID := range []string{"", "0", "invalid", "-1", "18446744073709551616"} {
		t.Run("path="+pathID, func(t *testing.T) {
			authorizer := &recordingAccessAuthorizer{allowedID: 7}
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodGet, "/testees?id=7", nil)
			c.Request.URL.RawQuery = "id=7"
			c.Params = gin.Params{{Key: "id", Value: pathID}}
			c.Set("user_claims", &pkgmiddleware.UserClaims{UserID: "9"})
			TesteeAccessMiddleware(authorizer, "id")(c)
			if rec.Code != http.StatusBadRequest || !c.IsAborted() {
				t.Fatalf("status=%d aborted=%v body=%s", rec.Code, c.IsAborted(), rec.Body.String())
			}
			if len(authorizer.calls) != 0 {
				t.Fatalf("invalid path reached authorizer: %v", authorizer.calls)
			}
		})
	}
}

func TestTesteeAccessMiddlewarePreservesQueryOnlyReports(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, query string
		allowedID   uint64
		status      int
		called      bool
	}{
		{"linked", "?testee_id=7", 7, http.StatusNoContent, true},
		{"unlinked", "?testee_id=8", 7, http.StatusForbidden, true},
		{"missing", "", 7, http.StatusBadRequest, false},
		{"malformed", "?testee_id=invalid", 7, http.StatusBadRequest, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			authorizer := &recordingAccessAuthorizer{allowedID: tc.allowedID}
			router := gin.New()
			router.Use(func(c *gin.Context) { c.Set("user_claims", &pkgmiddleware.UserClaims{UserID: "9"}); c.Next() })
			router.GET("/reports", TesteeAccessMiddleware(authorizer, "testee_id"), func(c *gin.Context) { c.Status(http.StatusNoContent) })
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/reports"+tc.query, nil))
			if rec.Code != tc.status {
				t.Fatalf("status=%d want=%d body=%s", rec.Code, tc.status, rec.Body.String())
			}
			if (len(authorizer.calls) == 1) != tc.called {
				t.Fatalf("authorizer calls=%v want called=%v", authorizer.calls, tc.called)
			}
		})
	}
}
