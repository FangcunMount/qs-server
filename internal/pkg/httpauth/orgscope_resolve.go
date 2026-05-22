package httpauth

import (
	"net/http"

	pkgmiddleware "github.com/FangcunMount/qs-server/internal/pkg/middleware"
	"github.com/FangcunMount/qs-server/internal/pkg/orgscope"
	"github.com/gin-gonic/gin"
)

// ResolveOrgScopeMiddleware resolves QS business org_id from QS membership data
// and writes it into gin context. JWT org_id claims are intentionally ignored.
func ResolveOrgScopeMiddleware(resolve orgscope.ResolveFunc) gin.HandlerFunc {
	if resolve == nil {
		resolve = orgscope.FixedResolver(orgscope.DefaultOrgID)
	}
	return func(c *gin.Context) {
		claims := pkgmiddleware.GetUserClaims(c)
		if claims == nil || claims.UserID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
			c.Abort()
			return
		}
		userID := GetUserID(c)
		if userID == 0 {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user id format"})
			c.Abort()
			return
		}

		requested := orgscope.RequestedOrgIDFromHTTP(c)
		orgID, err := resolve(c.Request.Context(), userID, requested)
		if err != nil || orgID == 0 {
			c.JSON(orgscope.HTTPStatusForResolveError(err), gin.H{"error": "organization scope could not be resolved"})
			c.Abort()
			return
		}
		applyResolvedOrgScope(c, claims, orgID)
		c.Next()
	}
}

func applyResolvedOrgScope(c *gin.Context, claims *pkgmiddleware.UserClaims, orgID uint64) {
	c.Set(OrgIDKey, orgID)
	tenantDomain := tenantDomainFromClaims(claims)
	setSecurityProjection(c, claims, tenantDomain, orgID, true)
}
