package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	assessmentApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	reportqueryjourney "github.com/FangcunMount/qs-server/internal/apiserver/application/journey/reportquery"
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/request"
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/response"
)

// GetAssessmentOutcome 获取 outcome 测评详情。
// @Summary 获取 outcome 测评详情
// @Description 根据 ID 获取测评，响应使用 model/primary_score/level 投影
// @Tags Evaluation-Assessment-Outcome
// @Produce json
// @Param id path string true "测评ID"
// @Success 200 {object} core.Response{data=response.AssessmentOutcomeResponse}
// @Failure 429 {object} core.ErrResponse
// @Router /api/v2/evaluations/assessments/{id} [get]
func (h *EvaluationHandler) GetAssessmentOutcome(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		h.BadRequestResponse(c, "无效的测评ID", err)
		return
	}
	orgID, operatorUserID, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	result, err := h.protectedQueryService.GetAssessmentOutcome(c.Request.Context(), protectedScope(orgID, operatorUserID), id)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, response.NewAssessmentOutcomeResponse(result))
}

// ListAssessmentsOutcome 查询 outcome 测评列表。
// @Summary 查询 outcome 测评列表
// @Description 分页查询测评列表，响应使用 model/primary_score/level 投影
// @Tags Evaluation-Assessment-Outcome
// @Produce json
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(10)
// @Param status query string false "状态筛选"
// @Param testee_id query string false "受试者ID筛选"
// @Success 200 {object} core.Response{data=response.AssessmentOutcomeListResponse}
// @Failure 429 {object} core.ErrResponse
// @Router /api/v2/evaluations/assessments [get]
func (h *EvaluationHandler) ListAssessmentsOutcome(c *gin.Context) {
	orgID, operatorUserID, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	var req request.ListAssessmentsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		h.BadRequestResponse(c, "请求参数无效", err)
		return
	}
	var testeeID *uint64
	if req.TesteeID > 0 {
		testeeID = &req.TesteeID
	}
	dto := assessmentApp.ListAssessmentsDTO{
		Page:     req.Page,
		PageSize: req.PageSize,
		TesteeID: testeeID,
		Status:   req.Status,
	}
	result, err := h.protectedQueryService.ListAssessmentsOutcome(c.Request.Context(), protectedScope(orgID, operatorUserID), dto)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, response.NewAssessmentOutcomeListResponse(result))
}

// GetReportOutcome 获取 outcome 测评报告。
// @Summary 获取 outcome 测评报告
// @Description 获取指定测评的解读报告，响应使用 model/primary_score/level 投影
// @Tags Evaluation-Report-Outcome
// @Produce json
// @Param id path string true "测评ID"
// @Success 200 {object} core.Response{data=response.ReportOutcomeResponse}
// @Failure 429 {object} core.ErrResponse
// @Router /api/v2/evaluations/assessments/{id}/report [get]
func (h *EvaluationHandler) GetReportOutcome(c *gin.Context) {
	id, scope, err := h.parseProtectedAssessmentQuery(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	result, err := h.reportQueryJourney.GetReportOutcome(c.Request.Context(), reportQueryScope(scope), id)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, response.NewReportOutcomeResponse(result))
}

// ListReportsOutcome 查询 outcome 报告列表。
// @Summary 查询 outcome 报告列表
// @Description 查询指定受试者的报告列表，响应使用 model/primary_score/level 投影
// @Tags Evaluation-Report-Outcome
// @Produce json
// @Param testee_id query string true "受试者ID"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(10)
// @Success 200 {object} core.Response{data=response.ReportOutcomeListResponse}
// @Failure 429 {object} core.ErrResponse
// @Router /api/v2/evaluations/reports [get]
func (h *EvaluationHandler) ListReportsOutcome(c *gin.Context) {
	orgID, operatorUserID, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	var req request.ListReportsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		h.BadRequestResponse(c, "请求参数无效", err)
		return
	}
	dto := reportqueryjourney.ListQuery{
		TesteeID: req.TesteeID,
		Page:     req.Page,
		PageSize: req.PageSize,
	}
	result, err := h.reportQueryJourney.ListReportsOutcome(c.Request.Context(), reportqueryjourney.Scope{OrgID: orgID, OperatorUserID: operatorUserID}, dto)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, response.NewReportOutcomeListResponse(result))
}
