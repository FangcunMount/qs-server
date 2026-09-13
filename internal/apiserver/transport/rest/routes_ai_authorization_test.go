package rest

import (
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	restmiddleware "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/middleware"
	"github.com/gin-gonic/gin"
)

func aiRouteSnapshotMiddleware(admin bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		permissions := []authz.Permission{{Resource: "qs:evaluation:collection:reports", Action: "audit", Mode: authz.AuthorizationModeUnconditional}}
		if admin {
			permissions = []authz.Permission{{Resource: "qs:*:*:*", Action: "*", Mode: authz.AuthorizationModeUnconditional}}
		}
		snapshot := &authz.Snapshot{Permissions: permissions}
		c.Set(restmiddleware.AuthzSnapshotKey, snapshot)
		c.Set(restmiddleware.OrgIDKey, uint64(12))
		c.Set(restmiddleware.UserIDKey, uint64(34))
		c.Request = c.Request.WithContext(authz.WithSnapshot(c.Request.Context(), snapshot))
		c.Next()
	}
}
