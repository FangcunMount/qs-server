package httpauth

import (
	"fmt"
	"net/http"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	iamauth "github.com/FangcunMount/qs-server/internal/pkg/iamauth"
	"github.com/gin-gonic/gin"
)

const AuthzSnapshotKey = "authz_snapshot"

// AuthzSnapshotMiddleware loads the IAM authorization snapshot for the parsed user.
//
// This generic transport helper does not persist operator role projections; apiserver
// keeps that process-specific behavior in its own middleware.
func AuthzSnapshotMiddleware(loader *iamauth.SnapshotLoader) gin.HandlerFunc {
	return func(c *gin.Context) {
		if loader == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "authorization snapshot loader not configured"})
			c.Abort()
			return
		}
		tenantID := GetTenantID(c)
		userIDStr := GetUserIDStr(c)
		if tenantID == "" || userIDStr == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "tenant_id and user identity are required for authorization"})
			c.Abort()
			return
		}
		snap, err := loader.Load(c.Request.Context(), tenantID, userIDStr)
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": fmt.Sprintf("failed to load authorization snapshot: %v", err)})
			c.Abort()
			return
		}
		c.Set(AuthzSnapshotKey, snap)
		c.Request = c.Request.WithContext(authz.WithSnapshot(c.Request.Context(), snap))
		c.Next()
	}
}

func GetAuthzSnapshot(c *gin.Context) *authz.Snapshot {
	v, ok := c.Get(AuthzSnapshotKey)
	if !ok {
		return nil
	}
	s, _ := v.(*authz.Snapshot)
	return s
}
