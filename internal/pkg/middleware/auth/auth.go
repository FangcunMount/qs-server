package auth

import (
	"github.com/gin-gonic/gin"
)

// AuthStrategy 认证策略接口，实现认证功能
type AuthStrategy interface {
	// AuthFunc 返回一个认证中间件函数
	AuthFunc() gin.HandlerFunc
}

// AuthOperator 认证操作器，用于在不同的认证策略之间切换
type AuthOperator struct {
	strategy AuthStrategy
}

// SetStrategy 设置认证策略
func (operator *AuthOperator) SetStrategy(strategy AuthStrategy) {
	operator.strategy = strategy
}

// AuthFunc 执行资源认证
func (operator *AuthOperator) AuthFunc() gin.HandlerFunc {
	return operator.strategy.AuthFunc()
}
