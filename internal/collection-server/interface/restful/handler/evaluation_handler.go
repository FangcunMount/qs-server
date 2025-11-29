package handler

import (
	"net/http"
	"strconv"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/evaluation"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/pkg/core"
	"github.com/gin-gonic/gin"
)

// TesteeIDKey 受试者ID在context中的key
const TesteeIDKey = "testee_id"

// GetTesteeID 从 gin.Context 获取受试者ID
func GetTesteeID(c *gin.Context) uint64 {
	val, exists := c.Get(TesteeIDKey)
	if !exists {
		return 0
	}
	if id, ok := val.(uint64); ok {
		return id
	}
	return 0
}

// EvaluationHandler 测评处理器
type EvaluationHandler struct {
	queryService *evaluation.QueryService
}

// NewEvaluationHandler 创建测评处理器
func NewEvaluationHandler(queryService *evaluation.QueryService) *EvaluationHandler {
	return &EvaluationHandler{
		queryService: queryService,
	}
}

// GetMyAssessment 获取我的测评详情
// @Summary 获取我的测评详情
// @Description 根据测评ID获取测评详情
// @Tags 测评
// @Produce json
// @Param id path int true "测评ID"
// @Success 200 {object} evaluation.AssessmentDetailResponse
// @Failure 400 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Security Bearer
// @Router /api/v1/assessments/{id} [get]
func (h *EvaluationHandler) GetMyAssessment(c *gin.Context) {
	testeeID := GetTesteeID(c)
	if testeeID == 0 {
		core.WriteResponse(c, errors.WithCode(code.ErrTokenInvalid, "testee not authenticated"), nil)
		return
	}

	idStr := c.Param("id")
	assessmentID, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		core.WriteResponse(c, errors.WithCode(code.ErrBind, "invalid assessment id"), nil)
		return
	}

	result, err := h.queryService.GetMyAssessment(c.Request.Context(), testeeID, assessmentID)
	if err != nil {
		core.WriteResponse(c, errors.WithCode(code.ErrDatabase, "get assessment failed: %v", err), nil)
		return
	}

	if result == nil {
		core.WriteResponse(c, errors.WithCode(code.ErrPageNotFound, "assessment not found"), nil)
		return
	}

	c.JSON(http.StatusOK, result)
}

// ListMyAssessments 获取我的测评列表
// @Summary 获取我的测评列表
// @Description 分页获取当前用户的测评列表
// @Tags 测评
// @Produce json
// @Param status query string false "状态筛选"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} evaluation.ListAssessmentsResponse
// @Failure 400 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Security Bearer
// @Router /api/v1/assessments [get]
func (h *EvaluationHandler) ListMyAssessments(c *gin.Context) {
	testeeID := GetTesteeID(c)
	if testeeID == 0 {
		core.WriteResponse(c, errors.WithCode(code.ErrTokenInvalid, "testee not authenticated"), nil)
		return
	}

	var req evaluation.ListAssessmentsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		core.WriteResponse(c, errors.WithCode(code.ErrBind, "bind query failed: %v", err), nil)
		return
	}

	result, err := h.queryService.ListMyAssessments(c.Request.Context(), testeeID, &req)
	if err != nil {
		core.WriteResponse(c, errors.WithCode(code.ErrDatabase, "list assessments failed: %v", err), nil)
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetAssessmentScores 获取测评得分详情
// @Summary 获取测评得分详情
// @Description 获取测评的因子得分详情
// @Tags 测评
// @Produce json
// @Param id path int true "测评ID"
// @Success 200 {array} evaluation.FactorScoreResponse
// @Failure 400 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Security Bearer
// @Router /api/v1/assessments/{id}/scores [get]
func (h *EvaluationHandler) GetAssessmentScores(c *gin.Context) {
	testeeID := GetTesteeID(c)
	if testeeID == 0 {
		core.WriteResponse(c, errors.WithCode(code.ErrTokenInvalid, "testee not authenticated"), nil)
		return
	}

	idStr := c.Param("id")
	assessmentID, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		core.WriteResponse(c, errors.WithCode(code.ErrBind, "invalid assessment id"), nil)
		return
	}

	result, err := h.queryService.GetAssessmentScores(c.Request.Context(), testeeID, assessmentID)
	if err != nil {
		core.WriteResponse(c, errors.WithCode(code.ErrDatabase, "get scores failed: %v", err), nil)
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetAssessmentReport 获取测评报告
// @Summary 获取测评报告
// @Description 获取测评的解读报告
// @Tags 测评
// @Produce json
// @Param id path int true "测评ID"
// @Success 200 {object} evaluation.AssessmentReportResponse
// @Failure 400 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Security Bearer
// @Router /api/v1/assessments/{id}/report [get]
func (h *EvaluationHandler) GetAssessmentReport(c *gin.Context) {
	testeeID := GetTesteeID(c)
	if testeeID == 0 {
		core.WriteResponse(c, errors.WithCode(code.ErrTokenInvalid, "testee not authenticated"), nil)
		return
	}

	idStr := c.Param("id")
	assessmentID, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		core.WriteResponse(c, errors.WithCode(code.ErrBind, "invalid assessment id"), nil)
		return
	}

	result, err := h.queryService.GetAssessmentReport(c.Request.Context(), testeeID, assessmentID)
	if err != nil {
		core.WriteResponse(c, errors.WithCode(code.ErrDatabase, "get report failed: %v", err), nil)
		return
	}

	if result == nil {
		core.WriteResponse(c, errors.WithCode(code.ErrPageNotFound, "report not found"), nil)
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetFactorTrend 获取因子得分趋势
// @Summary 获取因子得分趋势
// @Description 获取指定因子的历史得分趋势
// @Tags 测评
// @Produce json
// @Param factor_code query string true "因子编码"
// @Param limit query int false "数据点数量" default(10)
// @Success 200 {array} evaluation.TrendPointResponse
// @Failure 400 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Security Bearer
// @Router /api/v1/assessments/trend [get]
func (h *EvaluationHandler) GetFactorTrend(c *gin.Context) {
	testeeID := GetTesteeID(c)
	if testeeID == 0 {
		core.WriteResponse(c, errors.WithCode(code.ErrTokenInvalid, "testee not authenticated"), nil)
		return
	}

	var req evaluation.GetFactorTrendRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		core.WriteResponse(c, errors.WithCode(code.ErrBind, "bind query failed: %v", err), nil)
		return
	}

	result, err := h.queryService.GetFactorTrend(c.Request.Context(), testeeID, &req)
	if err != nil {
		core.WriteResponse(c, errors.WithCode(code.ErrDatabase, "get trend failed: %v", err), nil)
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetHighRiskFactors 获取高风险因子
// @Summary 获取高风险因子
// @Description 获取指定测评的高风险因子列表
// @Tags 测评
// @Produce json
// @Param id path int true "测评ID"
// @Success 200 {array} evaluation.FactorScoreResponse
// @Failure 400 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Security Bearer
// @Router /api/v1/assessments/{id}/factors/high-risk [get]
func (h *EvaluationHandler) GetHighRiskFactors(c *gin.Context) {
	testeeID := GetTesteeID(c)
	if testeeID == 0 {
		core.WriteResponse(c, errors.WithCode(code.ErrTokenInvalid, "testee not authenticated"), nil)
		return
	}

	idStr := c.Param("id")
	assessmentID, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		core.WriteResponse(c, errors.WithCode(code.ErrBind, "invalid assessment id"), nil)
		return
	}

	result, err := h.queryService.GetHighRiskFactors(c.Request.Context(), testeeID, assessmentID)
	if err != nil {
		core.WriteResponse(c, errors.WithCode(code.ErrDatabase, "get high risk factors failed: %v", err), nil)
		return
	}

	c.JSON(http.StatusOK, result)
}
