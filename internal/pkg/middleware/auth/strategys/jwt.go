package strategys

import (
	ginjwt "github.com/appleboy/gin-jwt/v2"
	"github.com/gin-gonic/gin"

	auth "github.com/FangcunMount/qs-server/internal/pkg/middleware/auth"
)

// JWTStrategy 定义jwt bearer认证策略
type JWTStrategy struct {
	// 嵌入 GinJWT Middleware
	ginjwt.GinJWTMiddleware
}

var _ auth.AuthStrategy = &JWTStrategy{}

// NewJWTStrategy 创建jwt bearer认证策略
func NewJWTStrategy(gjwt ginjwt.GinJWTMiddleware) JWTStrategy {
	return JWTStrategy{gjwt}
}

// AuthFunc 定义jwt bearer认证策略为gin认证中间件
func (j JWTStrategy) AuthFunc() gin.HandlerFunc {
	// 返回jwt认证中间件
	return j.MiddlewareFunc()
}
