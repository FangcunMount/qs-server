package catalogpeek

import "github.com/gin-gonic/gin"

// Entry 声明式 L1 peek 注册项（传输层 admission 策略）。
type Entry struct {
	RouteMatch func(route string) bool
	HasCached  func(c *gin.Context) bool
}

// Registry catalog L1 peek 注册表。
type Registry struct {
	entries []Entry
}

// NewRegistry 创建 peek 注册表。
func NewRegistry() *Registry {
	return &Registry{}
}

// Register 注册 peek 项。
func (r *Registry) Register(entry Entry) {
	if r == nil || entry.HasCached == nil {
		return
	}
	r.entries = append(r.entries, entry)
}

// Peek 判断 GET 请求是否可由 L1 直接响应（无需占用 catalog 并发槽）。
func (r *Registry) Peek(c *gin.Context) bool {
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
