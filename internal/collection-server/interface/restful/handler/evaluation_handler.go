package handler

import (
	"strconv"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/evaluation"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/pkg/core"
	"github.com/gin-gonic/gin"
)

// EvaluationHandler 测评处理器
type EvaluationHandler struct {
	*BaseHandler
	queryService *evaluation.QueryService
}

// NewEvaluationHandler 创建测评处理器
func NewEvaluationHandler(queryService *evaluation.QueryService) *EvaluationHandler {
	return &EvaluationHandler{
		BaseHandler:  NewBaseHandler(),
		queryService: queryService,
	}
}

// GetMyAssessment 获取我的测评详情
// @Summary 获取我的测评详情
// @Description 根据测评ID获取测评详情
// @Tags 测评
// @Produce json
// @Param id path int true "测评ID"
// @Param testee_id query int true "受试者ID"
// @Success 200 {object} core.Response{data=evaluation.AssessmentDetailResponse}
// @Failure 400 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Security Bearer
// @Router /api/v1/assessments/{id} [get]
func (h *EvaluationHandler) GetMyAssessment(c *gin.Context) {
	// 从 query 参数获取 testee_id（监护关系验证已在中间件完成，或由业务逻辑验证）
	testeeIDStr := h.GetQueryParam(c, "testee_id")
	if testeeIDStr == "" {
		h.BadRequestResponse(c, "testee_id is required", nil)
		return
	}
	testeeID, err := strconv.ParseUint(testeeIDStr, 10, 64)
	if err != nil {
		h.BadRequestResponse(c, "invalid testee_id format", err)
		return
	}

	idStr := h.GetPathParam(c, "id")
	assessmentID, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		h.BadRequestResponse(c, "invalid assessment id", err)
		return
	}

	result, err := h.queryService.GetMyAssessment(c.Request.Context(), testeeID, assessmentID)
	if err != nil {
		h.InternalErrorResponse(c, "get assessment failed", err)
		return
	}

	if result == nil {
		h.NotFoundResponse(c, "assessment not found", nil)
		return
	}

	h.Success(c, result)
}

// ListMyAssessments 获取我的测评列表
// @Summary 获取我的测评列表
// @Description 分页获取当前用户的测评列表
// @Tags 测评
// @Produce json
// @Param testee_id query int true "受试者ID"
// @Param status query string false "状态筛选"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} core.Response{data=evaluation.ListAssessmentsResponse}
// @Failure 400 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Security Bearer
// @Router /api/v1/assessments [get]
func (h *EvaluationHandler) ListMyAssessments(c *gin.Context) {
	// 从 query 参数获取 testee_id
	testeeIDStr := h.GetQueryParam(c, "testee_id")
	if testeeIDStr == "" {
		h.BadRequestResponse(c, "testee_id is required", nil)
		return
	}
	testeeID, err := strconv.ParseUint(testeeIDStr, 10, 64)
	if err != nil {
		h.BadRequestResponse(c, "invalid testee_id format", err)
		return
	}

	var req evaluation.ListAssessmentsRequest
	if err := h.BindQuery(c, &req); err != nil {
		return
	}

	result, err := h.queryService.ListMyAssessments(c.Request.Context(), testeeID, &req)
	if err != nil {
		h.InternalErrorResponse(c, "list assessments failed", err)
		return
	}

	h.Success(c, result)
}

// GetAssessmentScores 获取测评得分详情
// @Summary 获取测评得分详情
// @Description 获取测评的因子得分详情
// @Tags 测评
// @Produce json
// @Param id path int true "测评ID"
// @Param testee_id query int true "受试者ID"
// @Success 200 {object} core.Response{data=[]evaluation.FactorScoreResponse}
// @Failure 400 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Security Bearer
// @Router /api/v1/assessments/{id}/scores [get]
func (h *EvaluationHandler) GetAssessmentScores(c *gin.Context) {
	// 从 query 参数获取 testee_id
	testeeIDStr := c.Query("testee_id")
	if testeeIDStr == "" {
		core.WriteResponse(c, errors.WithCode(code.ErrBind, "testee_id is required"), nil)
		return
	}
	testeeID, err := strconv.ParseUint(testeeIDStr, 10, 64)
	if err != nil {
		core.WriteResponse(c, errors.WithCode(code.ErrBind, "invalid testee_id format"), nil)
		return
	}

	idStr := h.GetPathParam(c, "id")
	assessmentID, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		h.BadRequestResponse(c, "invalid assessment id", err)
		return
	}

	result, err := h.queryService.GetAssessmentScores(c.Request.Context(), testeeID, assessmentID)
	if err != nil {
		h.InternalErrorResponse(c, "get scores failed", err)
		return
	}

	h.Success(c, result)
}

// GetAssessmentReport 获取测评报告
// @Summary 获取测评报告
// @Description 获取测评的解读报告
// @Tags 测评
// @Produce json
// @Param id path int true "测评ID"
// @Param testee_id query int true "受试者ID"
// @Success 200 {object} core.Response{data=evaluation.AssessmentReportResponse}
// @Failure 400 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Security Bearer
// @Router /api/v1/assessments/{id}/report [get]
func (h *EvaluationHandler) GetAssessmentReport(c *gin.Context) {
	// 从 query 参数获取 testee_id
	testeeIDStr := h.GetQueryParam(c, "testee_id")
	if testeeIDStr == "" {
		h.BadRequestResponse(c, "testee_id is required", nil)
		return
	}
	testeeID, err := strconv.ParseUint(testeeIDStr, 10, 64)
	if err != nil {
		h.BadRequestResponse(c, "invalid testee_id format", err)
		return
	}

	idStr := h.GetPathParam(c, "id")
	assessmentID, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		h.BadRequestResponse(c, "invalid assessment id", err)
		return
	}

	result, err := h.queryService.GetAssessmentReport(c.Request.Context(), testeeID, assessmentID)
	if err != nil {
		h.InternalErrorResponse(c, "get report failed", err)
		return
	}

	if result == nil {
		h.NotFoundResponse(c, "report not found", nil)
		return
	}

	h.Success(c, result)
}

// GetFactorTrend 获取因子得分趋势
// @Summary 获取因子得分趋势
// @Description 获取指定因子的历史得分趋势
// @Tags 测评
// @Produce json
// @Param testee_id query int true "受试者ID"
// @Param factor_code query string true "因子编码"
// @Param limit query int false "数据点数量" default(10)
// @Success 200 {object} core.Response{data=[]evaluation.TrendPointResponse}
// @Failure 400 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Security Bearer
// @Router /api/v1/assessments/trend [get]
func (h *EvaluationHandler) GetFactorTrend(c *gin.Context) {
	// 从 query 参数获取 testee_id
	testeeIDStr := h.GetQueryParam(c, "testee_id")
	if testeeIDStr == "" {
		h.BadRequestResponse(c, "testee_id is required", nil)
		return
	}
	testeeID, err := strconv.ParseUint(testeeIDStr, 10, 64)
	if err != nil {
		h.BadRequestResponse(c, "invalid testee_id format", err)
		return
	}

	var req evaluation.GetFactorTrendRequest
	if err := h.BindQuery(c, &req); err != nil {
		return
	}

	result, err := h.queryService.GetFactorTrend(c.Request.Context(), testeeID, &req)
	if err != nil {
		h.InternalErrorResponse(c, "get trend failed", err)
		return
	}

	h.Success(c, result)
}

// GetHighRiskFactors 获取高风险因子
// @Summary 获取高风险因子
// @Description 获取指定测评的高风险因子列表
// @Tags 测评
// @Produce json
// @Param id path int true "测评ID"
// @Param testee_id query int true "受试者ID"
// @Success 200 {object} core.Response{data=[]evaluation.FactorScoreResponse}
// @Failure 400 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Security Bearer
// @Router /api/v1/assessments/{id}/factors/high-risk [get]
func (h *EvaluationHandler) GetHighRiskFactors(c *gin.Context) {
	// 从 query 参数获取 testee_id
	testeeIDStr := h.GetQueryParam(c, "testee_id")
	if testeeIDStr == "" {
		h.BadRequestResponse(c, "testee_id is required", nil)
		return
	}
	testeeID, err := strconv.ParseUint(testeeIDStr, 10, 64)
	if err != nil {
		h.BadRequestResponse(c, "invalid testee_id format", err)
		return
	}

	idStr := h.GetPathParam(c, "id")
	assessmentID, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		h.BadRequestResponse(c, "invalid assessment id", err)
		return
	}

	result, err := h.queryService.GetHighRiskFactors(c.Request.Context(), testeeID, assessmentID)
	if err != nil {
		h.InternalErrorResponse(c, "get high risk factors failed", err)
		return
	}

	h.Success(c, result)
}
