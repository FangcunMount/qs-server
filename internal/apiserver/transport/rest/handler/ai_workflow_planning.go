package handler

import (
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/gin-gonic/gin"
	"net/http"
)

// Prepare godoc
// @Summary 准备 qs-ai 评测的完整版本清单与策略预算
// @Description 只读操作，需要当前机构解读审计权限；组织及操作人取认证上下文。不创建 Run、不预约额度、不调用模型，预算不表示当前可用额度。创建和启动另需明确确认。
// @Tags AI-Workflow-Management
// @Accept json
// @Produce json
// @Param body body app.EvaluationPlanQuery true "确认的套件、生成路线和语义评测路线"
// @Success 200 {object} core.Response{data=app.EvaluationPlan}
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 403 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 409 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /internal/v2/interpretation/ai-workflow/evaluations/prepare [post]
func (h *AIWorkflowManagementHandler) Prepare(c *gin.Context) {
	org, user, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8192)
	var query app.EvaluationPlanQuery
	if err := c.ShouldBindJSON(&query); err != nil {
		h.failure(c, app.ErrInvalid)
		return
	}
	value, err := h.service.Prepare(c.Request.Context(), app.DraftScope{OrganizationID: org, OperatorUserID: user}, query)
	if err != nil {
		h.failure(c, err)
		return
	}
	h.Success(c, value)
}
