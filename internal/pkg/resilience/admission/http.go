package admission

import (
	"github.com/gin-gonic/gin"
)

// NewHTTPMiddleware 按准入策略获取槽位，失败时执行 onReject 并中断请求链。
func NewHTTPMiddleware(strategy Strategy, onReject gin.HandlerFunc) gin.HandlerFunc {
	if strategy == nil {
		return func(c *gin.Context) { c.Next() }
	}
	return func(c *gin.Context) {
		release, _, err := strategy.Acquire(c.Request.Context())
		if err != nil {
			if onReject != nil {
				onReject(c)
			}
			c.Abort()
			return
		}
		defer release()
		c.Next()
	}
}
