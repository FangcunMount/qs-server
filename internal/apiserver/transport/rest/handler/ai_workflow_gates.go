package handler

import (
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/gin-gonic/gin"
	"strconv"
)

// PreviewGates godoc
// @Summary 预览 qs-ai 评测发布门槛
// @Description 需要当前机构解读审计权限及明确 Run 版本。仅返回冻结证据的门槛预览，不批准 Run 或发布配置；证据未完成或版本变化返回冲突。
// @Tags AI-Workflow-Management
// @Produce json
// @Param run_id path string true "评测 Run UUID"
// @Param expected_version query int true "当前 Run 版本"
// @Success 200 {object} core.Response{data=app.EvaluationGatePreview}
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 403 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 409 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /internal/v2/interpretation/ai-workflow/evaluations/{run_id}/gates [get]
func (h *AIWorkflowManagementHandler) PreviewGates(c *gin.Context) {
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
	value, err := h.service.PreviewGates(c.Request.Context(), scope, version)
	if err != nil {
		h.failure(c, err)
		return
	}
	h.Success(c, value)
}
