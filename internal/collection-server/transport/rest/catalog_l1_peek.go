package rest

import "github.com/gin-gonic/gin"

// catalogL1Peek 判断 catalog 请求是否可由进程内 L1 直接响应（无需占用并发槽）。
func (r *Router) catalogL1Peek(c *gin.Context) bool {
	if r == nil || r.container == nil {
		return false
	}
	registry := r.container.CatalogL1PeekRegistry()
	if registry == nil {
		return false
	}
	return registry.Peek(c)
}
