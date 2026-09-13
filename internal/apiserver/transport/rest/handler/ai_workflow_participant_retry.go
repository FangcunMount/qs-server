package handler

import (
	"net/http"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/gin-gonic/gin"
)

func (h *AIWorkflowParticipantHandler) scope(c *gin.Context) (app.DraftScope, bool) {
	org, user, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return app.DraftScope{}, false
	}
	return app.DraftScope{OrganizationID: org, OperatorUserID: user}, true
}

// Get godoc
// @Summary 查询 qs-ai 参与者当前执行
// @Description 当前机构 OrgAdmin 权限，返回原请求、当前 Run、版本、失败分类与重试资格；不读取报告正文或发起调用。
// @Tags AI-Workflow-Management
// @Produce json
// @Param session_id path string true "AI 会话 UUID"
// @Success 200 {object} core.Response{data=app.ParticipantExecution}
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 403 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /internal/v2/interpretation/ai-workflow/participants/{session_id} [get]
func (h *AIWorkflowParticipantHandler) Get(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	value, err := h.service.Get(c.Request.Context(), scope, c.Param("session_id"))
	if err != nil {
		NewAIWorkflowManagementHandler(nil).failure(c, err)
		return
	}
	h.Success(c, value)
}

// Retry godoc
// @Summary 显式重试 qs-ai 参与者解读
// @Description 当前机构 OrgAdmin 授权。要求原 Run/版本、稳定命令 UUID、理由和一次调用费用确认；未知结果需额外风险确认。AI 复查参与者当前权限，在同一事务预留新额度、保留原尝试并创建新 Run。超时只查询原命令，不自动重发。
// @Tags AI-Workflow-Management
// @Accept json
// @Produce json
// @Param session_id path string true "AI 会话 UUID"
// @Param body body app.ParticipantRetry true "重试确认"
// @Success 200 {object} core.Response{data=app.Receipt}
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 403 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 409 {object} core.ErrResponse
// @Failure 429 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /internal/v2/interpretation/ai-workflow/participants/{session_id}/retry [post]
func (h *AIWorkflowParticipantHandler) Retry(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8192)
	var command app.ParticipantRetry
	if err := h.BindJSON(c, &command); err != nil {
		return
	}
	value, err := h.service.Retry(c.Request.Context(), scope, c.Param("session_id"), command)
	if err != nil {
		NewAIWorkflowManagementHandler(nil).failure(c, err)
		return
	}
	h.Success(c, value)
}

// RetryReceipt godoc
// @Summary 查询原参与者重试命令回执
// @Description 当前机构 OrgAdmin 授权，仅原操作人可读取该命令；AI 仍复查原参与者当前访问权。不创建新的执行，404 不能证明在途原命令未提交。
// @Tags AI-Workflow-Management
// @Produce json
// @Param command_id path string true "原重试命令 UUID"
// @Success 200 {object} core.Response{data=app.Receipt}
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 403 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /internal/v2/interpretation/ai-workflow/participants/retry-commands/{command_id} [get]
func (h *AIWorkflowParticipantHandler) RetryReceipt(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	value, err := h.service.RetryReceipt(c.Request.Context(), scope, c.Param("command_id"))
	if err != nil {
		NewAIWorkflowManagementHandler(nil).failure(c, err)
		return
	}
	h.Success(c, value)
}
