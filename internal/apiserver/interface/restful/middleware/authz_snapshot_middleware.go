package middleware

import (
	"fmt"
	"net/http"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/actor/actorctx"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	domainoperator "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operator"
	iaminfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	iamauth "github.com/FangcunMount/qs-server/internal/pkg/iamauth"
	"github.com/gin-gonic/gin"
)

const (
	// AuthzSnapshotKey gin 上下文中 IAM 授权快照的键。
	AuthzSnapshotKey = "authz_snapshot"
)

// AuthzSnapshotMiddleware 加载 IAM GetAuthorizationSnapshot 并写入 gin 与 request context。
// 若当前请求已解析出 active operator，则顺手将 IAM roles 投影回本地 staff/operator 表。
func AuthzSnapshotMiddleware(loader *iamauth.SnapshotLoader, repo domainoperator.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		if loader == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error": "authorization snapshot loader not configured",
			})
			c.Abort()
			return
		}
		tenantID := GetTenantID(c)
		userIDStr := GetUserIDStr(c)
		if tenantID == "" || userIDStr == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "tenant_id and user identity are required for authorization",
			})
			c.Abort()
			return
		}
		snap, err := loader.Load(c.Request.Context(), tenantID, userIDStr)
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error": fmt.Sprintf("failed to load authorization snapshot: %v", err),
			})
			c.Abort()
			return
		}
		c.Set(AuthzSnapshotKey, snap)
		ctx := authz.WithSnapshot(c.Request.Context(), snap)
		ctx = actorctx.WithGrantingUserID(ctx, GetUserID(c))
		c.Request = c.Request.WithContext(ctx)
		if repo != nil {
			if op := GetCurrentOperator(c); op != nil {
				if _, err := iaminfra.PersistOperatorRolesProjectionFromSnapshot(c.Request.Context(), repo, op, snap); err != nil {
					logger.L(c.Request.Context()).Warnw("failed to persist operator roles projection from IAM snapshot",
						"org_id", op.OrgID(),
						"user_id", op.UserID(),
						"error", err.Error(),
					)
				}
			}
		}
		c.Next()
	}
}

// GetAuthzSnapshot 从 gin 读取 IAM 授权快照（可能为 nil）。
func GetAuthzSnapshot(c *gin.Context) *authz.Snapshot {
	v, ok := c.Get(AuthzSnapshotKey)
	if !ok {
		return nil
	}
	s, _ := v.(*authz.Snapshot)
	return s
}
