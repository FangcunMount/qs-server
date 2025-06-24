package middleware

import (
	"github.com/gin-gonic/gin"
)

// AuthStrategy 定义了用于执行资源认证的方法集
type AuthStrategy interface {
	AuthFunc() gin.HandlerFunc
}

// AuthOperator 用于在不同的认证策略之间切换
type AuthOperator struct {
	strategy AuthStrategy
}

// SetStrategy 用于设置另一个认证策略
func (operator *AuthOperator) SetStrategy(strategy AuthStrategy) {
	operator.strategy = strategy
}

// AuthFunc 执行资源认证
func (operator *AuthOperator) AuthFunc() gin.HandlerFunc {
	return operator.strategy.AuthFunc()
}
