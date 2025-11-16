package strategys

import (
	"encoding/base64"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/FangcunMount/compose-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/middleware"
	"github.com/FangcunMount/qs-server/internal/pkg/middleware/auth"
	"github.com/FangcunMount/qs-server/pkg/core"
)

// BasicStrategy 基础策略认证器
type BasicStrategy struct {
	compare func(username string, password string) bool
}

// 实现AuthStrategy接口
var _ auth.AuthStrategy = &BasicStrategy{}

// NewBasicStrategy 创建基础认证策略器
func NewBasicStrategy(compare func(username string, password string) bool) BasicStrategy {
	return BasicStrategy{
		compare: compare,
	}
}

// AuthFunc 定义基础认证策略器为gin认证中间件
func (b BasicStrategy) AuthFunc() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 获取Authorization头
		auth := strings.SplitN(c.Request.Header.Get("Authorization"), " ", 2)

		// 如果Authorization头格式不正确，返回错误
		if len(auth) != 2 || auth[0] != "Basic" {
			core.WriteResponse(
				c,
				errors.WithCode(code.ErrSignatureInvalid, "Authorization header format is wrong."),
				nil,
			)
			c.Abort()

			return
		}

		// 解码Authorization头
		payload, _ := base64.StdEncoding.DecodeString(auth[1])
		// 分割用户名和密码
		pair := strings.SplitN(string(payload), ":", 2)

		// 如果用户名和密码不匹配，返回错误
		if len(pair) != 2 || !b.compare(pair[0], pair[1]) {
			core.WriteResponse(
				c,
				errors.WithCode(code.ErrSignatureInvalid, "Authorization header format is wrong."),
				nil,
			)
			c.Abort()

			return
		}

		// 设置用户名到context
		c.Set(middleware.UsernameKey, pair[0])

		// 继续处理请求
		c.Next()
	}
}
