package middleware

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/FangcunMount/qs-server/internal/collection-server/infra/iam"
	"github.com/FangcunMount/qs-server/internal/pkg/httpauth"
	pkgmiddleware "github.com/FangcunMount/qs-server/internal/pkg/middleware"
	"github.com/FangcunMount/qs-server/internal/pkg/securityplane"
)

const (
	// UserIDKey is the legacy collection context key for numeric user ID.
	UserIDKey = httpauth.UserIDKey
	// ChildIDKey is the collection context key for verified child ID.
	ChildIDKey = "child_id"
	// TesteeIDKey is reserved for business lookup results.
	TesteeIDKey = "testee_id"
	// PrincipalKey stores the Security Control Plane principal projection.
	PrincipalKey = httpauth.PrincipalKey
	// TenantScopeKey stores the Security Control Plane tenant scope projection.
	TenantScopeKey = httpauth.TenantScopeKey
)

// UserIdentityMiddleware keeps collection legacy context keys while delegating
// identity projection to the shared HTTP auth runtime.
func UserIdentityMiddleware() gin.HandlerFunc {
	return httpauth.UserIdentityMiddleware()
}

// GuardianshipVerifier verifies guardian-child relationships.
type GuardianshipVerifier interface {
	IsGuardian(ctx gin.Context, userID, childID string) (bool, error)
}

// GuardianshipMiddleware verifies that the authenticated user is the guardian
// of the child referenced by the given query or path parameter.
func GuardianshipMiddleware(guardianshipSvc *iam.GuardianshipService, childIDParam string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if guardianshipSvc == nil || !guardianshipSvc.IsEnabled() {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error": "guardianship service not available",
			})
			c.Abort()
			return
		}

		claims := pkgmiddleware.GetUserClaims(c)
		if claims == nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "user not authenticated",
			})
			c.Abort()
			return
		}

		childIDStr := c.Query(childIDParam)
		if childIDStr == "" {
			childIDStr = c.Param(childIDParam)
		}
		if childIDStr == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": fmt.Sprintf("missing required parameter: %s", childIDParam),
			})
			c.Abort()
			return
		}

		isGuardian, err := guardianshipSvc.IsGuardian(c.Request.Context(), claims.UserID, childIDStr)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": fmt.Sprintf("failed to verify guardianship: %v", err),
			})
			c.Abort()
			return
		}
		if !isGuardian {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "you are not the guardian of this child",
			})
			c.Abort()
			return
		}

		childID, err := strconv.ParseUint(childIDStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": fmt.Sprintf("invalid child id format: %s", childIDStr),
			})
			c.Abort()
			return
		}
		c.Set(ChildIDKey, childID)

		c.Next()
	}
}

// OptionalGuardianshipMiddleware verifies guardianship only when the child ID
// parameter is present; unavailable IAM dependencies degrade open as before.
func OptionalGuardianshipMiddleware(guardianshipSvc *iam.GuardianshipService, childIDParam string) gin.HandlerFunc {
	return func(c *gin.Context) {
		childIDStr := c.Query(childIDParam)
		if childIDStr == "" {
			childIDStr = c.Param(childIDParam)
		}
		if childIDStr == "" {
			c.Next()
			return
		}

		if guardianshipSvc == nil || !guardianshipSvc.IsEnabled() {
			c.Next()
			return
		}

		claims := pkgmiddleware.GetUserClaims(c)
		if claims == nil {
			c.Next()
			return
		}

		isGuardian, err := guardianshipSvc.IsGuardian(c.Request.Context(), claims.UserID, childIDStr)
		if err != nil {
			c.Next()
			return
		}
		if isGuardian {
			childID, _ := strconv.ParseUint(childIDStr, 10, 64)
			c.Set(ChildIDKey, childID)
		}

		c.Next()
	}
}

// GetUserID returns the numeric collection user ID from gin.Context.
func GetUserID(c *gin.Context) uint64 {
	return httpauth.GetUserID(c)
}

// GetChildID returns the verified child ID from gin.Context.
func GetChildID(c *gin.Context) uint64 {
	val, exists := c.Get(ChildIDKey)
	if !exists {
		return 0
	}
	id, _ := val.(uint64)
	return id
}

// GetPrincipal returns the Security Control Plane principal projection.
func GetPrincipal(c *gin.Context) (securityplane.Principal, bool) {
	return httpauth.GetPrincipal(c)
}

// GetTenantScope returns the Security Control Plane tenant scope projection.
func GetTenantScope(c *gin.Context) (securityplane.TenantScope, bool) {
	return httpauth.GetTenantScope(c)
}
