package httpauth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	pkgmiddleware "github.com/FangcunMount/qs-server/internal/pkg/middleware"
	"github.com/FangcunMount/qs-server/internal/pkg/orgscope"
	"github.com/gin-gonic/gin"
)

func TestResolveOrgScopeMiddlewareWritesQSOrgScope(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_claims", &pkgmiddleware.UserClaims{
			UserID:       "42",
			TenantDomain: "fangcun",
			OrgID:        "99",
		})
		c.Next()
	})
	router.Use(UserIdentityMiddleware())
	router.Use(ResolveOrgScopeMiddleware(orgscope.FixedResolver(88)))
	router.Use(RequireOrgScopeMiddleware())
	router.GET("/", func(c *gin.Context) {
		if got := GetOrgID(c); got != 88 {
			t.Fatalf("org_id = %d, want 88 from QS resolver", got)
		}
		principal, ok := GetPrincipal(c)
		if !ok || !principal.HasOrgID || principal.OrgID != 88 {
			t.Fatalf("principal = %#v, want org 88", principal)
		}
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
}
