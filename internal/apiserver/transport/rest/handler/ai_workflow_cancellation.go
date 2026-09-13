package handler

import (
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/gin-gonic/gin"
)

// Cancel godoc
// @Summary 取消或废弃 qs-ai 评测
// @Description 复用当前机构 OrgAdmin 权限，要求当前版本、理由、明确确认和 discard 决定。AI 在事务内判断取消资格并保留执行和审核历史；已派发或未知调用须先完成或处置。超时先回读原任务，不自动重试。
// @Tags AI-Workflow-Management
// @Accept json
// @Produce json
// @Param run_id path string true "评测 Run UUID"
// @Param body body app.EvaluationCancel true "取消确认"
// @Success 200 {object} core.Response{data=app.EvaluationState}
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 403 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 409 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /internal/v2/interpretation/ai-workflow/evaluations/{run_id}/cancel [post]
func (h *AIWorkflowManagementHandler) Cancel(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	var command app.EvaluationCancel
	if err := h.BindJSON(c, &command); err != nil {
		return
	}
	value, err := h.service.Cancel(c.Request.Context(), scope, command)
	if err != nil {
		h.failure(c, err)
		return
	}
	h.Success(c, value)
}
