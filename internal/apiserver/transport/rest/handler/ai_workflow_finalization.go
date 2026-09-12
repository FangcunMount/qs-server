package handler

import (
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/gin-gonic/gin"
)

// Finalize godoc
// @Summary 完成 qs-ai 评测最终评审
// @Description 需要当前机构 OrgAdmin 权限、预览版本、预期结果和明确确认。AI 在事务内重新计算门槛并记录批准或拒绝；不发布配置。超时后先回读状态，不自动重试。
// @Tags AI-Workflow-Management
// @Accept json
// @Produce json
// @Param run_id path string true "评测 Run UUID"
// @Param body body app.EvaluationFinalize true "最终评审确认"
// @Success 200 {object} core.Response{data=app.EvaluationState}
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 403 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 409 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /internal/v2/interpretation/ai-workflow/evaluations/{run_id}/finalize [post]
func (h *AIWorkflowManagementHandler) Finalize(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	var command app.EvaluationFinalize
	if err := h.BindJSON(c, &command); err != nil {
		return
	}
	value, err := h.service.Finalize(c.Request.Context(), scope, command)
	if err != nil {
		h.failure(c, err)
		return
	}
	h.Success(c, value)
}
