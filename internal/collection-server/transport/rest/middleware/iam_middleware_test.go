package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	pkgmiddleware "github.com/FangcunMount/qs-server/internal/pkg/middleware"
	"github.com/FangcunMount/qs-server/internal/pkg/securityplane"
)

func TestUserIdentityMiddlewareKeepsLegacyKeysAndSecurityProjection(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_claims", &pkgmiddleware.UserClaims{
			UserID:       "42",
			AccountID:    "acct-1",
			TenantDomain: "fangcun",
			OrgID:        "88",
			SessionID:    "sess-1",
			TokenID:      "tok-1",
			Roles:        []string{"guardian"},
			AMR:          []string{"pwd"},
		})
		c.Next()
	})
	router.Use(UserIdentityMiddleware())
	router.GET("/", func(c *gin.Context) {
		if got := GetUserID(c); got != 42 {
			t.Fatalf("legacy user id = %d, want 42", got)
		}
		principal, ok := GetPrincipal(c)
		if !ok {
			t.Fatal("principal projection missing")
		}
		if principal.Kind != securityplane.PrincipalKindUser || principal.Source != securityplane.PrincipalSourceHTTPJWT {
			t.Fatalf("unexpected principal kind/source: %#v", principal)
		}
		if principal.UserID != "42" || principal.AccountID != "acct-1" || principal.HasOrgID {
			t.Fatalf("unexpected principal: %#v", principal)
		}
		if principal.TenantDomain != "fangcun" {
			t.Fatalf("unexpected tenant domain: %#v", principal)
		}
		c.Status(http.StatusNoContent)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
}

func TestGetProfileIDReturnsVerifiedProfileID(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(ProfileIDKey, uint64(7))

	if got := GetProfileID(c); got != 7 {
		t.Fatalf("profile id = %d, want 7", got)
	}
}
