package handler

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/middleware"
	"github.com/FangcunMount/qs-server/pkg/core"
	"github.com/gin-gonic/gin"
)

const (
	// DefaultOrgID 默认机构ID（单租户场景）
	DefaultOrgID uint64 = 1
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

// GetOrgIDWithDefault 从上下文获取组织ID，如果为空则返回默认值
func (h *BaseHandler) GetOrgIDWithDefault(c *gin.Context) uint64 {
	orgID := h.GetOrgID(c)
	if orgID == 0 {
		return DefaultOrgID
	}
	return orgID
}

// GetRoles 从上下文获取用户角色列表
func (h *BaseHandler) GetRoles(c *gin.Context) []string {
	return middleware.GetRoles(c)
}

// HasRole 检查用户是否拥有指定角色
func (h *BaseHandler) HasRole(c *gin.Context, role string) bool {
	return middleware.HasRole(c, role)
}
