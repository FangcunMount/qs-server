package handler

import (
	"strconv"

	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	evaluationoperator "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/operator"
	"github.com/gin-gonic/gin"
)

// GetAssessmentProgress 查询单个测评的流程进度，不包含专业结果。
// @Summary 查询测评进度
// @Tags Evaluation-Progress
// @Produce json
// @Param id path string true "测评ID"
// @Success 200 {object} core.Response{data=evaluationoperator.Progress}
// @Failure 403 {object} core.ErrResponse
// @Failure 429 {object} core.ErrResponse
// @Router /api/v1/evaluations/assessment-progress/{id} [get]
func (h *EvaluationOperatorHandler) GetAssessmentProgress(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		h.BadRequestResponse(c, "无效的测评ID", err)
		return
	}
	org, user, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	query, ok := h.protectedQueryService.(evaluationoperator.ProgressQueryService)
	if !ok {
		h.Error(c, evalerrors.ModuleNotConfigured("测评进度查询未配置"))
		return
	}
	result, err := query.GetProgress(c.Request.Context(), evaluationoperator.Actor{OrgID: org, OperatorUserID: user}, id)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, result)
}

// ListAssessmentProgress 在业务访问范围内分页查询测评进度。
// @Summary 查询测评进度列表
// @Tags Evaluation-Progress
// @Produce json
// @Param page query int false "页码"
// @Param page_size query int false "每页数量"
// @Param testee_id query string false "受试者ID"
// @Param status query string false "流程状态"
// @Success 200 {object} core.Response{data=evaluationoperator.ProgressList}
// @Failure 403 {object} core.ErrResponse
// @Failure 429 {object} core.ErrResponse
// @Router /api/v1/evaluations/assessment-progress [get]
func (h *EvaluationOperatorHandler) ListAssessmentProgress(c *gin.Context) {
	org, user, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	var request struct {
		Page     int     `form:"page" binding:"omitempty,min=1"`
		PageSize int     `form:"page_size" binding:"omitempty,min=1,max=100"`
		TesteeID *uint64 `form:"testee_id"`
		Status   string  `form:"status"`
	}
	if err = c.ShouldBindQuery(&request); err != nil {
		h.BadRequestResponse(c, "无效的查询参数", err)
		return
	}
	query, ok := h.protectedQueryService.(evaluationoperator.ProgressQueryService)
	if !ok {
		h.Error(c, evalerrors.ModuleNotConfigured("测评进度查询未配置"))
		return
	}
	result, err := query.ListProgress(c.Request.Context(), evaluationoperator.Actor{OrgID: org, OperatorUserID: user}, evaluationoperator.ListQuery{Page: request.Page, PageSize: request.PageSize, TesteeID: request.TesteeID, Status: request.Status})
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, result)
}
