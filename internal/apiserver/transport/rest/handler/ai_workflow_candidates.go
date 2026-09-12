package handler

import (
	"strconv"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/gin-gonic/gin"
)

// ListCandidates godoc
// @Summary 查询 qs-ai 评测候选列表
// @Description 需要当前机构解读审计权限。列表最多 35 条，版本用于详情读取与审核；列表不代表质量通过。
// @Tags AI-Workflow-Management
// @Produce json
// @Param run_id path string true "评测 Run UUID"
// @Success 200 {object} core.Response{data=app.EvaluationCandidateIndex}
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 403 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 409 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /internal/v2/interpretation/ai-workflow/evaluations/{run_id}/candidates [get]
func (h *AIWorkflowManagementHandler) ListCandidates(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	value, err := h.service.ListCandidates(c.Request.Context(), scope)
	if err != nil {
		h.failure(c, err)
		return
	}
	h.Success(c, value)
}

// GetCandidate godoc
// @Summary 读取 qs-ai 候选的冻结审核证据
// @Description 需要当前机构解读审计权限及当前 Run 版本。正文和语义输出是保留原字节的 JSON 字符串，供展示与摘要校验；证据未完成或版本变化返回冲突。
// @Tags AI-Workflow-Management
// @Produce json
// @Param run_id path string true "评测 Run UUID"
// @Param candidate_id path string true "候选 ID"
// @Param expected_version query int true "列表返回的 Run 版本"
// @Success 200 {object} core.Response{data=app.EvaluationCandidateEvidence}
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 403 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 409 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /internal/v2/interpretation/ai-workflow/evaluations/{run_id}/candidates/{candidate_id} [get]
func (h *AIWorkflowManagementHandler) GetCandidate(c *gin.Context) {
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
	value, err := h.service.GetCandidate(c.Request.Context(), scope, app.CandidateQuery{CandidateID: c.Param("candidate_id"), ExpectedVersion: version})
	if err != nil {
		h.failure(c, err)
		return
	}
	h.Success(c, value)
}
