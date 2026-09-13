package handler

import (
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/gin-gonic/gin"
	"strconv"
)

// ListUnknowns godoc
// @Summary 查询 qs-ai 未处置的未知调用明细
// @Description 需要当前机构解读审计权限及当前任务版本。返回原调用、失败阶段和预算，不返回模型正文；活动执行或版本变化返回冲突。允许替代仅为快照提示，处置时重新校验。
// @Tags AI-Workflow-Management
// @Produce json
// @Param run_id path string true "评测 Run UUID"
// @Param expected_version query int true "列表返回的 Run 版本"
// @Success 200 {object} core.Response{data=app.EvaluationUnknownIndex}
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 403 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 409 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /internal/v2/interpretation/ai-workflow/evaluations/{run_id}/result-unknown [get]
func (h *AIWorkflowManagementHandler) ListUnknowns(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	versions := c.Request.URL.Query()["expected_version"]
	if len(versions) != 1 {
		h.failure(c, app.ErrInvalid)
		return
	}
	version, err := strconv.ParseInt(versions[0], 10, 64)
	if err != nil || version < 1 {
		h.failure(c, app.ErrInvalid)
		return
	}
	value, err := h.service.ListUnknowns(c.Request.Context(), scope, version)
	if err != nil {
		h.failure(c, err)
		return
	}
	h.Success(c, value)
}
