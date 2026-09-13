package handler

import (
	"strconv"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/gin-gonic/gin"
)

// List godoc
// @Summary 查询 qs-ai 原生评测任务目录
// @Description 复用当前机构解读审计权限，按状态和创建时间分页，仅返回摘要。组织及操作者取认证上下文；游标绑定机构和筛选，不授予写权限，也不返回模型原始输出。
// @Tags AI-Workflow-Management
// @Produce json
// @Param status query string false "requested/collecting/blocked/awaiting_review/approved/rejected/canceled"
// @Param limit query int false "每页 1–100，默认 20"
// @Param cursor query string false "上一页游标，改变筛选时清空"
// @Success 200 {object} core.Response{data=app.EvaluationCatalogPage}
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 403 {object} core.ErrResponse
// @Failure 409 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /internal/v2/interpretation/ai-workflow/evaluations [get]
func (h *AIWorkflowManagementHandler) List(c *gin.Context) {
	org, user, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	if len(c.Request.URL.RawQuery) > 8192 {
		h.failure(c, app.ErrInvalid)
		return
	}
	limit := 20
	if value, exists := c.GetQuery("limit"); exists {
		parsed, parseErr := strconv.Atoi(value)
		if parseErr != nil || parsed < 1 || parsed > 100 {
			h.failure(c, app.ErrInvalid)
			return
		}
		limit = parsed
	}
	page, err := h.service.List(c.Request.Context(), app.DraftScope{OrganizationID: org, OperatorUserID: user}, app.EvaluationCatalogQuery{Status: c.Query("status"), Limit: limit, Cursor: c.Query("cursor")})
	if err != nil {
		h.failure(c, err)
		return
	}
	h.Success(c, page)
}
