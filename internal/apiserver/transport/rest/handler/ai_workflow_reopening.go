package handler

import (
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/gin-gonic/gin"
)

// ReopenReview godoc
// @Summary 重开 qs-ai 评测语义复核
// @Description 需要当前机构 OrgAdmin 权限、当前版本、理由和明确确认。AI 在事务内核对重开资格并保留旧审核和门槛，最多三轮；不调用模型或发布配置。超时后先回读状态，不自动重试。
// @Tags AI-Workflow-Management
// @Accept json
// @Produce json
// @Param run_id path string true "评测 Run UUID"
// @Param body body app.EvaluationReopen true "重开确认"
// @Success 200 {object} core.Response{data=app.EvaluationState}
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 403 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 409 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /internal/v2/interpretation/ai-workflow/evaluations/{run_id}/reopen-review [post]
func (h *AIWorkflowManagementHandler) ReopenReview(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	var command app.EvaluationReopen
	if err := h.BindJSON(c, &command); err != nil {
		return
	}
	value, err := h.service.ReopenReview(c.Request.Context(), scope, command)
	if err != nil {
		h.failure(c, err)
		return
	}
	h.Success(c, value)
}
