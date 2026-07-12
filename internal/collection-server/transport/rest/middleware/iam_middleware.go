package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	testeeapp "github.com/FangcunMount/qs-server/internal/collection-server/application/testee"
	"github.com/FangcunMount/qs-server/internal/collection-server/infra/iam"
	"github.com/FangcunMount/qs-server/internal/pkg/httpauth"
	pkgmiddleware "github.com/FangcunMount/qs-server/internal/pkg/middleware"
	"github.com/FangcunMount/qs-server/internal/pkg/securityplane"
)

type TesteeProfileResolver interface {
	GetTestee(context.Context, uint64) (*testeeapp.TesteeResponse, error)
}
type ActiveProfileLinkChecker interface {
	IsEnabled() bool
	HasActiveProfileLink(context.Context, string, string) (bool, error)
}

const (
	UserIDKey    = httpauth.UserIDKey
	ProfileIDKey = "profile_id"
	TesteeIDKey  = "testee_id"
	PrincipalKey = httpauth.PrincipalKey
)

func UserIdentityMiddleware() gin.HandlerFunc {
	return httpauth.UserIdentityMiddleware()
}

// TesteeProfileLinkMiddleware resolves a QS testee to its IAM profile and
// verifies that the authenticated user owns an active link to that profile.
func TesteeProfileLinkMiddleware(testees TesteeProfileResolver, links ActiveProfileLinkChecker, testeeParam string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if testees == nil || links == nil || !links.IsEnabled() {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "profile link service not available"})
			c.Abort()
			return
		}
		claims := pkgmiddleware.GetUserClaims(c)
		if claims == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
			c.Abort()
			return
		}
		raw := c.Query(testeeParam)
		if raw == "" {
			raw = c.Param(testeeParam)
		}
		id, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || id == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid testee_id"})
			c.Abort()
			return
		}
		testee, err := testees.GetTestee(c.Request.Context(), id)
		if err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": "testee access denied"})
			c.Abort()
			return
		}
		if testee == nil || testee.IAMProfileID == "" {
			c.JSON(http.StatusForbidden, gin.H{"error": "testee profile is not linked"})
			c.Abort()
			return
		}
		allowed, err := links.HasActiveProfileLink(c.Request.Context(), claims.UserID, testee.IAMProfileID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to verify profile link"})
			c.Abort()
			return
		}
		if !allowed {
			c.JSON(http.StatusForbidden, gin.H{"error": "you do not have access to this profile"})
			c.Abort()
			return
		}
		c.Set(TesteeIDKey, id)
		c.Next()
	}
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
