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
			UserID:   "42",
			TenantID: "88",
			Roles:    []string{"operator"},
		})
		c.Next()
	})
	router.Use(UserIdentityMiddleware())
	router.GET("/", func(c *gin.Context) {
		if got := GetUserID(c); got != 42 {
			t.Fatalf("user_id = %d, want 42", got)
		}
		if got := GetUserIDStr(c); got != "42" {
			t.Fatalf("user_id_str = %q, want 42", got)
		}
		if got := GetTenantID(c); got != "88" {
			t.Fatalf("tenant_id = %q, want 88", got)
		}
		if got := GetOrgID(c); got != 88 {
			t.Fatalf("org_id = %d, want 88", got)
		}
		if got := GetRoles(c); len(got) != 1 || got[0] != "operator" {
			t.Fatalf("roles = %#v, want [operator]", got)
		}
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
}

func TestRequireNumericOrgScopeMiddlewareRejectsNonNumericTenant(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_claims", &pkgmiddleware.UserClaims{UserID: "42", TenantID: "tenant-alpha"})
		c.Next()
	})
	router.Use(UserIdentityMiddleware())
	router.Use(RequireNumericOrgScopeMiddleware())
	router.GET("/", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}
