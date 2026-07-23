package middleware

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/testeeaccess"
	"github.com/FangcunMount/qs-server/internal/pkg/httpauth"
	pkgmiddleware "github.com/FangcunMount/qs-server/internal/pkg/middleware"
	"github.com/FangcunMount/qs-server/internal/pkg/securityplane"
)

type TesteeAccessAuthorizer interface {
	Authorize(ctx context.Context, userID string, testeeID uint64) error
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

// TesteeAccessMiddleware verifies that the authenticated IAM User can
// represent the requested Testee before a protected report query is executed.
func TesteeAccessMiddleware(authorizer TesteeAccessAuthorizer, testeeParam string) gin.HandlerFunc {
	return func(c *gin.Context) {
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
		if authorizer == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "authorization temporarily unavailable"})
			c.Abort()
			return
		}
		if err := authorizer.Authorize(c.Request.Context(), claims.UserID, id); err != nil {
			if errors.Is(err, testeeaccess.ErrAccessDenied) {
				c.JSON(http.StatusForbidden, gin.H{"error": "assessment access denied"})
			} else {
				c.JSON(http.StatusServiceUnavailable, gin.H{"error": "authorization temporarily unavailable"})
			}
			c.Abort()
			return
		}
		c.Set(TesteeIDKey, id)
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
