package handler

import (
	"errors"
	"net/http"

	"github.com/FangcunMount/component-base/pkg/logger"
	appreportstatus "github.com/FangcunMount/qs-server/internal/collection-server/application/reportstatus"
	"github.com/FangcunMount/qs-server/pkg/core"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (h *BaseHandler) writeReportStatusError(c *gin.Context, operation string, err error) {
	if errors.Is(err, appreportstatus.ErrAssessmentAccess) ||
		status.Code(err) == codes.NotFound ||
		status.Code(err) == codes.PermissionDenied {
		h.NotFoundResponse(c, "assessment not found", nil)
		return
	}

	logger.L(c.Request.Context()).Errorw("report status dependency unavailable",
		"operation", operation,
		"error", err,
	)
	c.JSON(http.StatusServiceUnavailable, core.ErrResponse{
		Code:    http.StatusServiceUnavailable,
		Message: "report status temporarily unavailable",
	})
}
