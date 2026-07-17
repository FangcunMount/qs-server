package rest

import (
	"net/http"

	"github.com/FangcunMount/qs-server/internal/pkg/resilience/ratelimit"
	"github.com/FangcunMount/qs-server/pkg/core"
	"github.com/gin-gonic/gin"
)

// WriteServiceUnavailable 在并发槽位耗尽时快速失败，避免请求静默排队至客户端超时。
func WriteServiceUnavailable(c *gin.Context, retryAfterSeconds int) {
	if retryAfterSeconds <= 0 {
		retryAfterSeconds = 1
	}
	ratelimit.ApplyRetryAfterSeconds(c.Writer.Header(), retryAfterSeconds)
	c.JSON(http.StatusServiceUnavailable, core.ErrResponse{
		Code:    http.StatusServiceUnavailable,
		Message: "service is busy, please retry later",
	})
}
