package handler

import (
	"strconv"

	runquery "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runquery"
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/response"
	"github.com/gin-gonic/gin"
)

// EvaluationRunInternalHandler serves operating endpoints for evaluation runs.
type EvaluationRunInternalHandler struct {
	*BaseHandler
	runQueryService runquery.Service
}

// NewEvaluationRunInternalHandler creates an internal evaluation run handler.
func NewEvaluationRunInternalHandler(runQueryService runquery.Service) *EvaluationRunInternalHandler {
	return &EvaluationRunInternalHandler{
		BaseHandler:     &BaseHandler{},
		runQueryService: runQueryService,
	}
}

// ListRetryableFailed lists retryable failed evaluation runs for the current org.
// @Summary 查询可重试失败运行列表
// @Description operating 内部接口，按组织返回可重试的失败运行
// @Tags Evaluation-Run-Internal
// @Produce json
// @Param retryable query bool false "仅可重试" default(true)
// @Param limit query int false "返回条数" default(50)
// @Param cursor query int false "分页游标"
// @Success 200 {object} core.Response{data=response.RetryableFailedRunListResponse}
// @Router /internal/v1/evaluation-runs/failed [get]
func (h *EvaluationRunInternalHandler) ListRetryableFailed(c *gin.Context) {
	if c.Query("retryable") == "false" {
		h.Success(c, response.NewRetryableFailedRunListResponse(&runquery.RetryableFailedListResult{}))
		return
	}
	orgID, _, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	cursor, _ := strconv.ParseUint(c.Query("cursor"), 10, 64)
	result, err := h.runQueryService.ListRetryableFailed(c.Request.Context(), orgID, limit, cursor)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, response.NewRetryableFailedRunListResponse(result))
}
