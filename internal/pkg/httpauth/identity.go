package httpauth

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/FangcunMount/component-base/pkg/logger"
	pkgmiddleware "github.com/FangcunMount/qs-server/internal/pkg/middleware"
	"github.com/gin-gonic/gin"
)

const (
	UserIDKey    = "user_id"
	UserIDStrKey = "user_id_str"
	OrgIDKey     = "org_id"
	TenantIDKey  = "tenant_id"
	RolesKey     = "roles"
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

		c.Set(TenantIDKey, claims.TenantID)
		if claims.TenantID != "" {
			if orgID, err := strconv.ParseUint(claims.TenantID, 10, 64); err == nil {
				c.Set(OrgIDKey, orgID)
			}
		}

		if len(claims.Roles) > 0 {
			c.Set(RolesKey, claims.Roles)
		}

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
		c.Set(TenantIDKey, claims.TenantID)
		if claims.UserID != "" {
			if userID, err := strconv.ParseUint(claims.UserID, 10, 64); err == nil {
				c.Set(UserIDKey, userID)
			}
		}
		if claims.TenantID != "" {
			if orgID, err := strconv.ParseUint(claims.TenantID, 10, 64); err == nil {
				c.Set(OrgIDKey, orgID)
			}
		}
		if len(claims.Roles) > 0 {
			c.Set(RolesKey, claims.Roles)
		}

		c.Next()
	}
}

// RequireTenantIDMiddleware requires a non-empty IAM tenant_id claim.
func RequireTenantIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := pkgmiddleware.GetUserClaims(c)
		logger.L(c.Request.Context()).Debugw("RequireTenantIDMiddleware claims", "claims", claims)
		if claims == nil || claims.TenantID == "" {
			logger.L(c.Request.Context()).Errorw("RequireTenantIDMiddleware claims is nil or empty", "claims", claims)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id claim is required"})
			c.Abort()
			return
		}
		c.Next()
	}
}

// RequireNumericOrgScopeMiddleware requires tenant_id to be parseable as a QS org_id.
func RequireNumericOrgScopeMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if GetOrgID(c) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "tenant_id must be a numeric organization id for QS"})
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

func GetTenantID(c *gin.Context) string {
	val, exists := c.Get(TenantIDKey)
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
