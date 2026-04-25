package handler

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/middleware"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/safeconv"
	"github.com/FangcunMount/qs-server/pkg/core"
	"github.com/gin-gonic/gin"
)

// BaseHandler 基础Handler结构
// 继承 core.BaseHandler 并添加 apiserver 特定的方法
type BaseHandler struct {
	*core.BaseHandler
}

// NewBaseHandler 创建基础Handler
func NewBaseHandler() *BaseHandler {
	return &BaseHandler{
		BaseHandler: core.NewBaseHandler(),
	}
}

// GetUserID 从上下文获取当前用户ID（需要认证中间件设置）
// 返回 string 类型的 UserID（兼容旧代码）
func (h *BaseHandler) GetUserID(c *gin.Context) (string, bool) {
	userID := middleware.GetUserIDStr(c)
	if userID == "" {
		return "", false
	}
	return userID, true
}

// GetUserIDUint64 从上下文获取当前用户ID（uint64 类型）
func (h *BaseHandler) GetUserIDUint64(c *gin.Context) (uint64, bool) {
	userID := middleware.GetUserID(c)
	if userID == 0 {
		return 0, false
	}
	return userID, true
}

// GetOrgID 从上下文获取组织ID（从 JWT TenantID 解析）
func (h *BaseHandler) GetOrgID(c *gin.Context) uint64 {
	return middleware.GetOrgID(c)
}

// RequireProtectedOrgID 获取受保护路由的机构范围。
func (h *BaseHandler) RequireProtectedOrgID(c *gin.Context) (int64, error) {
	orgID := h.GetOrgID(c)
	if orgID == 0 {
		return 0, errors.WithCode(code.ErrPermissionDenied, "protected route requires org scope from JWT")
	}
	resolvedID, err := safeconv.Uint64ToInt64(orgID)
	if err != nil {
		return 0, errors.WithCode(code.ErrPermissionDenied, "protected route org scope exceeds int64")
	}
	return resolvedID, nil
}

// RequireProtectedUserID 获取受保护路由的用户身份。
func (h *BaseHandler) RequireProtectedUserID(c *gin.Context) (int64, error) {
	userID, ok := h.GetUserIDUint64(c)
	if !ok || userID == 0 {
		return 0, errors.WithCode(code.ErrPermissionDenied, "protected route requires user identity from JWT")
	}
	resolvedID, err := safeconv.Uint64ToInt64(userID)
	if err != nil {
		return 0, errors.WithCode(code.ErrPermissionDenied, "protected route user identity exceeds int64")
	}
	return resolvedID, nil
}

// RequireProtectedScope 获取受保护路由的组织和用户信息。
func (h *BaseHandler) RequireProtectedScope(c *gin.Context) (int64, int64, error) {
	orgID, err := h.RequireProtectedOrgID(c)
	if err != nil {
		return 0, 0, err
	}
	userID, err := h.RequireProtectedUserID(c)
	if err != nil {
		return 0, 0, err
	}
	return orgID, userID, nil
}

// RequireProtectedOrgIDWithLegacy 在 JWT org 语义下兼容旧请求体/query 的 org_id。
func (h *BaseHandler) RequireProtectedOrgIDWithLegacy(c *gin.Context, legacyOrgID int64) (int64, error) {
	orgID, err := h.RequireProtectedOrgID(c)
	if err != nil {
		return 0, err
	}
	if legacyOrgID != 0 && legacyOrgID != orgID {
		return 0, errors.WithCode(code.ErrInvalidArgument, "org_id does not match JWT org scope")
	}
	return orgID, nil
}
