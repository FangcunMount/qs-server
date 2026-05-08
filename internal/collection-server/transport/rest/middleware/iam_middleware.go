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
	// ProfileIDKey is the collection context key for verified IAM ProfileID.
	ProfileIDKey = "profile_id"
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

// ProfileLinkMiddleware verifies that the authenticated user has an active
// link to the profile referenced by the given query or path parameter.
func ProfileLinkMiddleware(profileLinkSvc *iam.ProfileLinkService, profileIDParam string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if profileLinkSvc == nil || !profileLinkSvc.IsEnabled() {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error": "profile link service not available",
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

		profileIDStr := c.Query(profileIDParam)
		if profileIDStr == "" {
			profileIDStr = c.Param(profileIDParam)
		}
		if profileIDStr == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": fmt.Sprintf("missing required parameter: %s", profileIDParam),
			})
			c.Abort()
			return
		}

		hasActiveProfileLink, err := profileLinkSvc.HasActiveProfileLink(c.Request.Context(), claims.UserID, profileIDStr)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": fmt.Sprintf("failed to verify profile link: %v", err),
			})
			c.Abort()
			return
		}
		if !hasActiveProfileLink {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "you do not have access to this profile",
			})
			c.Abort()
			return
		}

		profileID, err := strconv.ParseUint(profileIDStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": fmt.Sprintf("invalid profile id format: %s", profileIDStr),
			})
			c.Abort()
			return
		}
		c.Set(ProfileIDKey, profileID)

		c.Next()
	}
}

// OptionalProfileLinkMiddleware verifies profile access only when the ProfileID
// parameter is present; unavailable IAM dependencies degrade open as before.
func OptionalProfileLinkMiddleware(profileLinkSvc *iam.ProfileLinkService, profileIDParam string) gin.HandlerFunc {
	return func(c *gin.Context) {
		profileIDStr := c.Query(profileIDParam)
		if profileIDStr == "" {
			profileIDStr = c.Param(profileIDParam)
		}
		if profileIDStr == "" {
			c.Next()
			return
		}

		if profileLinkSvc == nil || !profileLinkSvc.IsEnabled() {
			c.Next()
			return
		}

		claims := pkgmiddleware.GetUserClaims(c)
		if claims == nil {
			c.Next()
			return
		}

		hasActiveProfileLink, err := profileLinkSvc.HasActiveProfileLink(c.Request.Context(), claims.UserID, profileIDStr)
		if err != nil {
			c.Next()
			return
		}
		if hasActiveProfileLink {
			profileID, _ := strconv.ParseUint(profileIDStr, 10, 64)
			c.Set(ProfileIDKey, profileID)
		}

		c.Next()
	}
}

// GetUserID returns the numeric collection user ID from gin.Context.
func GetUserID(c *gin.Context) uint64 {
	return httpauth.GetUserID(c)
}

// GetProfileID returns the verified IAM ProfileID from gin.Context.
func GetProfileID(c *gin.Context) uint64 {
	val, exists := c.Get(ProfileIDKey)
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
