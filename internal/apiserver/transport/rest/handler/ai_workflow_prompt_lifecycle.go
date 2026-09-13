package handler

import (
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/gin-gonic/gin"
)

// GetLifecycle godoc
// @Summary 读取 qs-ai 草稿当前修订和冻结状态
// @Description 需要当前机构解读审计权限；不接受历史修订选择器，不返回他人冻结命令回执。读取不授权后续修改。
// @Tags AI-Workflow-Prompt-Drafts
// @Produce json
// @Param draft_id path string true "草稿 UUID"
// @Success 200 {object} core.Response{data=app.PromptDraftLifecycle}
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 403 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 409 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /internal/v2/interpretation/ai-workflow/prompt-drafts/{draft_id}/lifecycle [get]
func (h *AIWorkflowPromptDraftHandler) GetLifecycle(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	if _, exists := c.Request.URL.Query()["revision"]; exists {
		h.failure(c, app.ErrInvalid)
		return
	}
	value, err := h.service.GetLifecycle(c.Request.Context(), scope, c.Param("draft_id"))
	if err != nil {
		h.failure(c, err)
		return
	}
	h.Success(c, value)
}
