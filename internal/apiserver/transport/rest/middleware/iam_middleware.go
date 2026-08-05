package middleware

import (
	"fmt"
	"net/http"

	"github.com/FangcunMount/component-base/pkg/errors"
	operatorapp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/operator"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/httpauth"
	"github.com/FangcunMount/qs-server/internal/pkg/orgscope"
	"github.com/FangcunMount/qs-server/internal/pkg/safeconv"
	"github.com/FangcunMount/qs-server/internal/pkg/securityplane"
	"github.com/gin-gonic/gin"
)

const (
	UserIDKey          = httpauth.UserIDKey
	UserIDStrKey       = httpauth.UserIDStrKey
	OrgIDKey           = httpauth.OrgIDKey
	TenantDomainKey    = httpauth.TenantDomainKey
	CurrentOperatorKey = "current_operator"
	PrincipalKey       = httpauth.PrincipalKey
	OrgScopeKey        = httpauth.OrgScopeKey
)

func UserIdentityMiddleware() gin.HandlerFunc {
	return httpauth.UserIdentityMiddleware()
}

func RequireTenantDomainMiddleware() gin.HandlerFunc {
	return httpauth.RequireTenantDomainMiddleware()
}

func ResolveOrgScopeMiddleware(resolve orgscope.ResolveFunc) gin.HandlerFunc {
	return httpauth.ResolveOrgScopeMiddleware(resolve)
}

func RequireOrgScopeMiddleware() gin.HandlerFunc {
	return httpauth.RequireOrgScopeMiddleware()
}

func RequireActiveOperatorMiddleware(checker operatorapp.ActiveOperatorChecker) gin.HandlerFunc {
	return func(c *gin.Context) {
		if GetCurrentOperator(c) != nil {
			c.Next()
			return
		}
		if checker == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "operator repository not configured"})
			c.Abort()
			return
		}
		orgID, err := safeconv.Uint64ToInt64(GetOrgID(c))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "organization scope exceeds int64"})
			c.Abort()
			return
		}
		uid := GetUserID(c)
		if orgID <= 0 || uid == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid organization or user scope"})
			c.Abort()
			return
		}
		userID, err := safeconv.Uint64ToInt64(uid)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "user scope exceeds int64"})
			c.Abort()
			return
		}
		op, err := checker.RequireActive(c.Request.Context(), orgID, userID)
		if err != nil {
			if errors.IsCode(err, code.ErrPermissionDenied) {
				c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
				c.Abort()
				return
			}
			if errors.IsCode(err, code.ErrUserNotFound) {
				c.JSON(http.StatusForbidden, gin.H{"error": "operator not found in current organization"})
				c.Abort()
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("operator lookup failed: %v", err)})
			c.Abort()
			return
		}
		c.Set(CurrentOperatorKey, op)
		c.Next()
	}
}

func GetUserID(c *gin.Context) uint64       { return httpauth.GetUserID(c) }
func GetUserIDStr(c *gin.Context) string    { return httpauth.GetUserIDStr(c) }
func GetOrgID(c *gin.Context) uint64        { return httpauth.GetOrgID(c) }
func GetTenantDomain(c *gin.Context) string { return httpauth.GetTenantDomain(c) }
func GetPrincipal(c *gin.Context) (securityplane.Principal, bool) {
	return httpauth.GetPrincipal(c)
}
func GetOrgScope(c *gin.Context) (securityplane.OrgScope, bool) {
	return httpauth.GetOrgScope(c)
}

func GetCurrentOperator(c *gin.Context) *operatorapp.OperatorResult {
	v, ok := c.Get(CurrentOperatorKey)
	if !ok {
		return nil
	}
	op, _ := v.(*operatorapp.OperatorResult)
	return op
}
