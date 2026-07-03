package catalogl1

import "github.com/gin-gonic/gin"

// PeekEntry 声明式 L1 peek 注册项。
type PeekEntry struct {
	RouteMatch func(route string) bool
	HasCached  func(c *gin.Context) bool
}

// PeekRegistry catalog L1 peek 注册表。
type PeekRegistry struct {
	entries []PeekEntry
}

// NewPeekRegistry 创建 peek 注册表。
func NewPeekRegistry() *PeekRegistry {
	return &PeekRegistry{}
}

// Register 注册 peek 项。
func (r *PeekRegistry) Register(entry PeekEntry) {
	if r == nil || entry.HasCached == nil {
		return
	}
	r.entries = append(r.entries, entry)
}

// Peek 判断请求是否可由 L1 直接响应。
func (r *PeekRegistry) Peek(c *gin.Context) bool {
	if r == nil || c == nil || c.Request == nil || c.Request.Method != "GET" {
		return false
	}
	route := c.FullPath()
	if route == "" {
		route = c.Request.URL.Path
	}
	for _, entry := range r.entries {
		if entry.RouteMatch != nil && entry.RouteMatch(route) && entry.HasCached(c) {
			return true
		}
	}
	return false
}
