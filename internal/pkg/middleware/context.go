package middleware

import (
	"github.com/gin-gonic/gin"

	"github.com/FangcunMount/component-base/pkg/log"
)

// UsernameKey 定义了在 gin 上下文中表示密钥所有者的键
const UsernameKey = "username"

// Context 是一个中间件，将公共前缀字段注入到 gin.Context 中
func Context() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(log.KeyRequestID, c.GetString(XRequestIDKey))
		c.Set(log.KeyUsername, c.GetString(UsernameKey))
		c.Next()
	}
}
