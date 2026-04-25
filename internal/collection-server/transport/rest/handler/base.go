package handler

import (
	"github.com/FangcunMount/qs-server/internal/collection-server/interface/restful/middleware"
	"github.com/FangcunMount/qs-server/pkg/core"
	"github.com/gin-gonic/gin"
)

// BaseHandler 基础Handler结构
// 继承 core.BaseHandler 并添加 collection-server 特定的方法
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
// 返回 uint64 类型的 UserID
func (h *BaseHandler) GetUserID(c *gin.Context) uint64 {
	return middleware.GetUserID(c)
}
