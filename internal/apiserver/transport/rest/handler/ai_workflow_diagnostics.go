package handler

import (
	"strconv"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/gin-gonic/gin"
)

func (h *AIWorkflowManagementHandler) executionQuery(c *gin.Context) (app.ExecutionQuery, bool) {
	values := c.Request.URL.Query()
	if len(c.Request.URL.RawQuery) > 8192 || len(values["expected_version"]) != 1 || len(values["cursor"]) > 1 || len(values["limit"]) > 1 {
		h.failure(c, app.ErrInvalid)
		return app.ExecutionQuery{}, false
	}
	version, err := strconv.ParseInt(values.Get("expected_version"), 10, 64)
	limit := 20
	if raw, exists := values["limit"]; exists {
		parsed, parseErr := strconv.Atoi(raw[0])
		if parseErr != nil || parsed < 1 || parsed > 50 {
			h.failure(c, app.ErrInvalid)
			return app.ExecutionQuery{}, false
		}
		limit = parsed
	}
	if err != nil || version < 1 {
		h.failure(c, app.ErrInvalid)
		return app.ExecutionQuery{}, false
	}
	return app.ExecutionQuery{ExpectedVersion: version, Cursor: values.Get("cursor"), Limit: limit}, true
}

// ListExecutions godoc
// @Summary 查询 qs-ai 任务的原执行记录
// @Description 复用当前机构审计权限，读取指定任务版本的执行摘要，包含未形成候选的失败及未知结果。摘要不返回模型正文，不触发模型或恢复。
// @Tags AI-Workflow-Management
// @Produce json
// @Param run_id path string true "评测 Run UUID"
// @Param expected_version query int true "当前任务版本"
// @Param limit query int false "每页 1–50，默认 20"
// @Param cursor query string false "上一页游标"
// @Success 200 {object} core.Response{data=app.ExecutionPage}
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 403 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 409 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /internal/v2/interpretation/ai-workflow/evaluations/{run_id}/executions [get]
func (h *AIWorkflowManagementHandler) ListExecutions(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	query, ok := h.executionQuery(c)
	if !ok {
		return
	}
	page, err := h.service.ListExecutions(c.Request.Context(), scope, query)
	if err != nil {
		h.failure(c, err)
		return
	}
	h.Success(c, page)
}

// GetExecutionOutput godoc
// @Summary 读取 qs-ai 原执行输出及失败诊断
// @Description 当前机构审计权限与任务版本校验。raw_output/normalized_output 为 Base64 原始字节，附 SHA256；空正文不代表执行成功，不重试原调用。
// @Tags AI-Workflow-Management
// @Produce json
// @Param run_id path string true "评测 Run UUID"
// @Param execution_id path string true "执行标识"
// @Param expected_version query int true "当前任务版本"
// @Success 200 {object} core.Response{data=app.ExecutionOutput}
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 403 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 409 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /internal/v2/interpretation/ai-workflow/evaluations/{run_id}/executions/{execution_id}/output [get]
func (h *AIWorkflowManagementHandler) GetExecutionOutput(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	query, ok := h.executionQuery(c)
	if !ok {
		return
	}
	value, err := h.service.GetExecutionOutput(c.Request.Context(), scope, query.ExpectedVersion, c.Param("execution_id"))
	if err != nil {
		h.failure(c, err)
		return
	}
	h.Success(c, value)
}
