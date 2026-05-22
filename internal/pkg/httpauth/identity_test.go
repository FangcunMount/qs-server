package httpauth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	pkgmiddleware "github.com/FangcunMount/qs-server/internal/pkg/middleware"
	"github.com/gin-gonic/gin"
)

func TestUserIdentityMiddlewareProjectsClaimsToGinContext(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_claims", &pkgmiddleware.UserClaims{
			UserID:       "42",
			TenantDomain: "fangcun",
			OrgID:        "88",
			Roles:        []string{"operator"},
		})
		c.Next()
	})
	router.Use(UserIdentityMiddleware())
	router.GET("/", func(c *gin.Context) {
		if got := GetUserID(c); got != 42 {
			t.Fatalf("user_id = %d, want 42", got)
		}
		if got := GetTenantDomain(c); got != "fangcun" {
			t.Fatalf("tenant_domain = %q, want fangcun", got)
		}
		if got := GetOrgID(c); got != 0 {
			t.Fatalf("org_id = %d, want 0 before QS org resolver", got)
		}
		if got := GetRoles(c); len(got) != 1 || got[0] != "operator" {
			t.Fatalf("roles = %#v, want [operator]", got)
		}
		principal, ok := GetPrincipal(c)
		if !ok {
			t.Fatal("expected security principal projection")
		}
		if principal.UserID != "42" || principal.TenantDomain != "fangcun" || principal.HasOrgID {
			t.Fatalf("principal = %#v, want user 42 domain fangcun without org", principal)
		}
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
}

func TestRequireOrgScopeMiddlewareRejectsMissingOrg(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_claims", &pkgmiddleware.UserClaims{UserID: "42", TenantDomain: "fangcun"})
		c.Next()
	})
	router.Use(UserIdentityMiddleware())
	router.Use(RequireOrgScopeMiddleware())
	router.GET("/", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}
