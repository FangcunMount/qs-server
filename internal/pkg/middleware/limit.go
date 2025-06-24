package middleware

import (
	"errors"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// ErrLimitExceeded 定义了限制超出错误
var ErrLimitExceeded = errors.New("Limit exceeded")

// Limit 如果达到限制，则丢弃（HTTP 状态 429）请求
func Limit(maxEventsPerSec float64, maxBurstSize int) gin.HandlerFunc {
	limiter := rate.NewLimiter(rate.Limit(maxEventsPerSec), maxBurstSize)

	return func(c *gin.Context) {
		if limiter.Allow() {
			c.Next()

			return
		}

		// Limit reached
		_ = c.Error(ErrLimitExceeded)
		c.AbortWithStatus(429)
	}
}
