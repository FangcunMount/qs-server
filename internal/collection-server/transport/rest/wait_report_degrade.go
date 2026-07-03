package rest

import (
	"net/http"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/evaluation"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/reportstatus"
	"github.com/FangcunMount/qs-server/internal/pkg/ratelimit"
	"github.com/FangcunMount/qs-server/pkg/core"
	"github.com/gin-gonic/gin"
)

// WriteDegradedWaitReport 在 wait-report HTTP 槽位不足时立即返回 pending 状态。
func WriteDegradedWaitReport(c *gin.Context, retryAfterSeconds int) {
	if retryAfterSeconds <= 0 {
		retryAfterSeconds = 5
	}
	retryAfterMs := retryAfterSeconds * 1000
	ratelimit.ApplyRetryAfterSeconds(c.Writer.Header(), retryAfterSeconds)
	c.JSON(http.StatusOK, core.Response{
		Code:    0,
		Message: "success",
		Data: reportstatus.ToPublicAssessmentStatus(&evaluation.AssessmentStatusResponse{
			Status:          "processing",
			Stage:           "queued",
			Message:         "系统繁忙，报告生成中",
			NextPollAfterMs: retryAfterMs,
		}),
	})
}
