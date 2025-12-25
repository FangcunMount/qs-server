package handler

import (
	"context"
	"strconv"
	"time"

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
// @Description 获取测评的因子得分详情。响应中的每个因子得分包含 max_score（最大分，可选）字段
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
// @Description 获取测评的解读报告。响应字段说明：
// @Description - dimensions（维度列表）：只包含 is_show = true 的因子维度，每个维度包含 factor_code（因子编码）、factor_name（因子名称）、raw_score（原始分）、max_score（最大分，可选）、risk_level（风险等级）、description（解读描述）、suggestion（维度建议，字符串）字段
// @Description - suggestions（建议列表）：报告级别的建议列表，每个建议包含 category（分类）、content（内容）、factor_code（关联因子编码，可选）字段
// @Tags 测评
// @Produce json
// @Param id path int true "测评ID"
// @Success 200 {object} core.Response{data=evaluation.AssessmentReportResponse}
// @Failure 400 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Security Bearer
// @Router /api/v1/assessments/{id}/report [get]
func (h *EvaluationHandler) GetAssessmentReport(c *gin.Context) {
	idStr := h.GetPathParam(c, "id")
	assessmentID, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		h.BadRequestResponse(c, "invalid assessment id", err)
		return
	}

	result, err := h.queryService.GetAssessmentReport(c.Request.Context(), assessmentID)
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

// WaitReport 长轮询等待报告生成
// @Summary 长轮询等待报告生成
// @Description 等待测评报告生成，支持长轮询机制。如果报告已生成则立即返回，否则等待最多 timeout 秒
// @Tags 测评
// @Produce json
// @Param id path int true "测评ID"
// @Param testee_id query int true "受试者ID"
// @Param timeout query int false "超时时间（秒）" default(15) minimum(5) maximum(60)
// @Success 200 {object} core.Response{data=evaluation.AssessmentStatusResponse}
// @Failure 400 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Security Bearer
// @Router /api/v1/assessments/{id}/wait-report [get]
func (h *EvaluationHandler) WaitReport(c *gin.Context) {
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

	// 解析超时参数
	timeoutStr := c.DefaultQuery("timeout", "15")
	timeoutSeconds, err := strconv.Atoi(timeoutStr)
	if err != nil || timeoutSeconds < 5 || timeoutSeconds > 60 {
		timeoutSeconds = 15
	}
	timeout := time.Duration(timeoutSeconds) * time.Second

	ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
	defer cancel()

	// 1. 快速检查一次状态
	result, err := h.queryService.GetMyAssessment(ctx, testeeID, assessmentID)
	if err == nil && result != nil {
		// 如果已经完成，立即返回
		if result.Status == "interpreted" || result.Status == "failed" {
			var totalScore *float64
			var riskLevel *string
			if result.TotalScore != 0 {
				ts := result.TotalScore
				totalScore = &ts
			}
			if result.RiskLevel != "" {
				rl := result.RiskLevel
				riskLevel = &rl
			}
			h.Success(c, &evaluation.AssessmentStatusResponse{
				Status:     result.Status,
				TotalScore: totalScore,
				RiskLevel:  riskLevel,
				UpdatedAt:  time.Now().Unix(),
			})
			return
		}
	}

	// 2. 短轮询：每1秒检查一次状态
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// 超时或客户端断开
			h.Success(c, &evaluation.AssessmentStatusResponse{
				Status:    "pending",
				UpdatedAt: time.Now().Unix(),
			})
			return

		case <-ticker.C:
			// 定期轮询状态
			result, err := h.queryService.GetMyAssessment(ctx, testeeID, assessmentID)
			if err == nil && result != nil {
				if result.Status == "interpreted" || result.Status == "failed" {
					var totalScore *float64
					var riskLevel *string
					if result.TotalScore != 0 {
						ts := result.TotalScore
						totalScore = &ts
					}
					if result.RiskLevel != "" {
						rl := result.RiskLevel
						riskLevel = &rl
					}
					h.Success(c, &evaluation.AssessmentStatusResponse{
						Status:     result.Status,
						TotalScore: totalScore,
						RiskLevel:  riskLevel,
						UpdatedAt:  time.Now().Unix(),
					})
					return
				}
			}
		}
	}
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

// GetMyAssessmentByAnswerSheetID 通过答卷ID获取测评详情
// @Summary 通过答卷ID获取测评详情
// @Description 根据答卷ID获取对应的测评详情
// @Tags 答卷
// @Produce json
// @Param id path int true "答卷ID"
// @Success 200 {object} core.Response{data=evaluation.AssessmentDetailResponse}
// @Failure 400 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Security Bearer
// @Router /api/v1/answersheets/{id}/assessment [get]
func (h *EvaluationHandler) GetMyAssessmentByAnswerSheetID(c *gin.Context) {
	idStr := h.GetPathParam(c, "id")
	answerSheetID, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		h.BadRequestResponse(c, "invalid answer sheet id", err)
		return
	}

	result, err := h.queryService.GetMyAssessmentByAnswerSheetID(c.Request.Context(), answerSheetID)
	if err != nil {
		h.InternalErrorResponse(c, "get assessment by answer sheet failed", err)
		return
	}

	if result == nil {
		h.NotFoundResponse(c, "assessment not found", nil)
		return
	}

	h.Success(c, result)
}
