package middleware

import (
	operatorapp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/operator"
	"github.com/FangcunMount/qs-server/internal/pkg/httpauth"
	"github.com/FangcunMount/qs-server/internal/pkg/orgscope"
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
