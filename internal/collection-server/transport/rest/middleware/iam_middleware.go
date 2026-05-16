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
	UserIDKey     = httpauth.UserIDKey
	ProfileIDKey  = "profile_id"
	TesteeIDKey   = "testee_id"
	PrincipalKey  = httpauth.PrincipalKey
	OrgScopeKey   = httpauth.OrgScopeKey
)

func UserIdentityMiddleware() gin.HandlerFunc {
	return httpauth.UserIdentityMiddleware()
}

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

func GetUserID(c *gin.Context) uint64 {
	return httpauth.GetUserID(c)
}

func GetProfileID(c *gin.Context) uint64 {
	val, exists := c.Get(ProfileIDKey)
	if !exists {
		return 0
	}
	id, _ := val.(uint64)
	return id
}

func GetPrincipal(c *gin.Context) (securityplane.Principal, bool) {
	return httpauth.GetPrincipal(c)
}

func GetOrgScope(c *gin.Context) (securityplane.OrgScope, bool) {
	return httpauth.GetOrgScope(c)
}
