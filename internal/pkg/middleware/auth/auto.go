package auth

import (
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/pkg/core"
)

// authHeaderCount 认证头数量
const authHeaderCount = 2

// AutoStrategy 自动认证策略器
type AutoStrategy struct {
	// 基础策略认证器
	basic AuthStrategy
	// JWT策略认证器
	jwt AuthStrategy
}

// 实现AuthStrategy接口
var _ AuthStrategy = &AutoStrategy{}

// NewAutoStrategy 创建自动认证策略器
func NewAutoStrategy(basic, jwt AuthStrategy) AutoStrategy {
	return AutoStrategy{
		basic: basic,
		jwt:   jwt,
	}
}

// AuthFunc 定义自动认证策略器为gin认证中间件
func (a AutoStrategy) AuthFunc() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 创建认证操作器
		operator := AuthOperator{}
		// 获取Authorization头
		authHeader := strings.SplitN(c.Request.Header.Get("Authorization"), " ", 2)

		// 如果Authorization头格式不正确，返回错误
		if len(authHeader) != authHeaderCount {
			core.WriteResponse(
				c,
				errors.WithCode(code.ErrInvalidAuthHeader, "Authorization header format is wrong."),
				nil,
			)
			c.Abort()

			return
		}

		// 根据Authorization头类型设置认证策略
		switch authHeader[0] {
		case "Basic":
			// 使用 Basic 认证器
			operator.SetStrategy(a.basic)
		case "Bearer":
			// 使用 JWT 认证器
			operator.SetStrategy(a.jwt)
		default:
			core.WriteResponse(c, errors.WithCode(code.ErrSignatureInvalid, "unrecognized Authorization header."), nil)
			c.Abort()

			return
		}

		// 执行认证
		operator.AuthFunc()(c)

		c.Next()
	}
}
