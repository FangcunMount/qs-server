package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	pkgmiddleware "github.com/FangcunMount/qs-server/internal/pkg/middleware"
	"github.com/FangcunMount/qs-server/internal/pkg/securityplane"
	"github.com/gin-gonic/gin"
)

func TestUserIdentityMiddlewareProjectsSecurityPrincipalAndScope(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_claims", &pkgmiddleware.UserClaims{
			UserID:    "42",
			AccountID: "account-1",
			TenantID:  "88",
			SessionID: "session-1",
			TokenID:   "token-1",
			Roles:     []string{"operator"},
			AMR:       []string{"pwd"},
		})
		c.Next()
	})
	router.Use(UserIdentityMiddleware())
	router.GET("/", func(c *gin.Context) {
		principal, ok := GetPrincipal(c)
		if !ok {
			t.Fatal("expected security principal projection")
		}
		if principal.Kind != securityplane.PrincipalKindUser || principal.Source != securityplane.PrincipalSourceHTTPJWT {
			t.Fatalf("principal kind/source = %#v", principal)
		}
		if principal.UserID != "42" || principal.AccountID != "account-1" || principal.TokenID != "token-1" {
			t.Fatalf("principal = %#v, want projected claims", principal)
		}
		scope, ok := GetTenantScope(c)
		if !ok {
			t.Fatal("expected tenant scope projection")
		}
		if !scope.HasNumericOrg || scope.OrgID != 88 {
			t.Fatalf("scope = %#v, want numeric org 88", scope)
		}
		if got := GetUserID(c); got != 42 {
			t.Fatalf("legacy user id = %d, want 42", got)
		}
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
}
