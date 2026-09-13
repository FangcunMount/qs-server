package handler

import (
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/gin-gonic/gin"
)

// Capacity godoc
// @Summary 查询 qs-ai 评测容量
// @Description 当前机构 OrgAdmin 权限；返回 UTC 日预算预留和活动任务占用。只读快照不保证下一次启动可用，启动在 AI 中原子检查。取消不退回日预算。
// @Tags AI-Workflow-Management
// @Produce json
// @Success 200 {object} core.Response{data=app.EvaluationCapacity}
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 403 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /internal/v2/interpretation/ai-workflow/evaluation-capacity [get]
func (h *AIWorkflowManagementHandler) Capacity(c *gin.Context) {
	org, user, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	result, err := h.service.Capacity(c.Request.Context(), app.DraftScope{OrganizationID: org, OperatorUserID: user})
	if err != nil {
		h.failure(c, err)
		return
	}
	h.Success(c, result)
}
