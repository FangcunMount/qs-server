package httpauth

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/FangcunMount/component-base/pkg/logger"
	pkgmiddleware "github.com/FangcunMount/qs-server/internal/pkg/middleware"
	"github.com/FangcunMount/qs-server/internal/pkg/securityplane"
	"github.com/FangcunMount/qs-server/internal/pkg/securityprojection"
	"github.com/gin-gonic/gin"
)

const (
	UserIDKey       = "user_id"
	UserIDStrKey    = "user_id_str"
	OrgIDKey        = "org_id"
	TenantDomainKey = "tenant_domain"
	RolesKey        = "roles"
	PrincipalKey    = "security_principal"
	OrgScopeKey     = "security_org_scope"
)

// UserIdentityMiddleware projects IAM JWT claims into gin.Context.
func UserIdentityMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := pkgmiddleware.GetUserClaims(c)
		if claims == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
			c.Abort()
			return
		}

		c.Set(UserIDStrKey, claims.UserID)
		if claims.UserID != "" {
			userID, err := strconv.ParseUint(claims.UserID, 10, 64)
			if err != nil {
				c.JSON(http.StatusUnauthorized, gin.H{"error": fmt.Sprintf("invalid user id format: %s", claims.UserID)})
				c.Abort()
				return
			}
			c.Set(UserIDKey, userID)
		}

		projectIdentityContext(c, claims)
		c.Next()
	}
}

// OptionalUserIdentityMiddleware projects IAM claims when present.
func OptionalUserIdentityMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := pkgmiddleware.GetUserClaims(c)
		if claims == nil {
			c.Next()
			return
		}

		c.Set(UserIDStrKey, claims.UserID)
		if claims.UserID != "" {
			if userID, err := strconv.ParseUint(claims.UserID, 10, 64); err == nil {
				c.Set(UserIDKey, userID)
			}
		}
		if len(claims.Roles) > 0 {
			c.Set(RolesKey, claims.Roles)
		}
		projectIdentityContext(c, claims)
		c.Next()
	}
}

// RequireTenantDomainMiddleware requires a non-empty IAM authorization domain claim.
func RequireTenantDomainMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := pkgmiddleware.GetUserClaims(c)
		logger.L(c.Request.Context()).Debugw("RequireTenantDomainMiddleware claims", "claims", claims)
		if claims == nil || tenantDomainFromClaims(claims) == "" {
			logger.L(c.Request.Context()).Errorw("RequireTenantDomainMiddleware missing tenant domain", "claims", claims)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant domain claim is required"})
			c.Abort()
			return
		}
		c.Next()
	}
}

// RequireOrgScopeMiddleware requires a resolvable QS business org_id in request context.
func RequireOrgScopeMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if GetOrgID(c) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "organization scope is required for QS business routes"})
			c.Abort()
			return
		}
		c.Next()
	}
}

func GetUserID(c *gin.Context) uint64 {
	val, exists := c.Get(UserIDKey)
	if !exists {
		return 0
	}
	id, _ := val.(uint64)
	return id
}

func GetUserIDStr(c *gin.Context) string {
	val, exists := c.Get(UserIDStrKey)
	if !exists {
		return ""
	}
	id, _ := val.(string)
	return id
}

func GetOrgID(c *gin.Context) uint64 {
	val, exists := c.Get(OrgIDKey)
	if !exists {
		return 0
	}
	id, _ := val.(uint64)
	return id
}

// GetTenantDomain returns the IAM authorization domain from gin.Context.
func GetTenantDomain(c *gin.Context) string {
	val, exists := c.Get(TenantDomainKey)
	if !exists {
		return ""
	}
	id, _ := val.(string)
	return id
}

func GetRoles(c *gin.Context) []string {
	val, exists := c.Get(RolesKey)
	if !exists {
		return nil
	}
	roles, _ := val.([]string)
	return roles
}

// GetPrincipal returns the Security Control Plane principal projection.
func GetPrincipal(c *gin.Context) (securityplane.Principal, bool) {
	val, exists := c.Get(PrincipalKey)
	if !exists {
		return securityplane.Principal{}, false
	}
	principal, ok := val.(securityplane.Principal)
	return principal, ok
}

// GetOrgScope returns the Security Control Plane org scope projection.
func GetOrgScope(c *gin.Context) (securityplane.OrgScope, bool) {
	val, exists := c.Get(OrgScopeKey)
	if !exists {
		return securityplane.OrgScope{}, false
	}
	scope, ok := val.(securityplane.OrgScope)
	return scope, ok
}

func projectIdentityContext(c *gin.Context, claims *pkgmiddleware.UserClaims) {
	tenantDomain := tenantDomainFromClaims(claims)
	c.Set(TenantDomainKey, tenantDomain)

	if len(claims.Roles) > 0 {
		c.Set(RolesKey, claims.Roles)
	}
	setSecurityProjection(c, claims, tenantDomain, 0, false)
}

func setSecurityProjection(c *gin.Context, claims *pkgmiddleware.UserClaims, tenantDomain string, orgID uint64, hasOrg bool) {
	if claims == nil {
		return
	}
	principal := securityprojection.PrincipalFromInput(securityprojection.PrincipalInput{
		Kind:         securityplane.PrincipalKindUser,
		Source:       securityplane.PrincipalSourceHTTPJWT,
		UserID:       claims.UserID,
		AccountID:    claims.AccountID,
		TenantDomain: tenantDomain,
		OrgID:        orgID,
		HasOrgID:     hasOrg,
		SessionID:    claims.SessionID,
		TokenID:      claims.TokenID,
		Roles:        claims.Roles,
		AMR:          claims.AMR,
	})
	c.Set(PrincipalKey, principal)
	c.Set(OrgScopeKey, securityprojection.OrgScopeFromIdentity(tenantDomain, orgID, hasOrg, ""))
}
