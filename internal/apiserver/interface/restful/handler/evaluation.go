package handler

import (
	"context"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	assessmentApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/engine"
	"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/request"
	"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/response"
)

// EvaluationHandler 评估模块 Handler
// 提供测评管理、得分查询、报告查询等 RESTful API
// 注意：此 Handler 服务于后台管理系统，不提供创建和提交测评接口
type EvaluationHandler struct {
	*BaseHandler
	managementService  assessmentApp.AssessmentManagementService
	reportQueryService assessmentApp.ReportQueryService
	scoreQueryService  assessmentApp.ScoreQueryService
	evaluationService  engine.Service
}

// NewEvaluationHandler 创建评估模块 Handler
func NewEvaluationHandler(
	managementService assessmentApp.AssessmentManagementService,
	reportQueryService assessmentApp.ReportQueryService,
	scoreQueryService assessmentApp.ScoreQueryService,
	evaluationService engine.Service,
) *EvaluationHandler {
	return &EvaluationHandler{
		BaseHandler:        &BaseHandler{},
		managementService:  managementService,
		reportQueryService: reportQueryService,
		scoreQueryService:  scoreQueryService,
		evaluationService:  evaluationService,
	}
}

// ============= Assessment 查询接口（后台管理）=============

// GetAssessment 获取测评详情
// @Summary 获取测评详情
// @Description 根据ID获取测评详细信息
// @Tags Evaluation-Assessment
// @Produce json
// @Param id path string true "测评ID"
// @Success 200 {object} core.Response{data=response.AssessmentResponse}
// @Router /api/v1/evaluations/assessments/{id} [get]
func (h *EvaluationHandler) GetAssessment(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		h.BadRequestResponse(c, "无效的测评ID", err)
		return
	}

	ctx := context.Background()
	result, err := h.managementService.GetByID(ctx, id)
	if err != nil {
		h.Error(c, err)
		return
	}

	h.Success(c, response.NewAssessmentResponse(result))
}

// ListAssessments 查询测评列表
// @Summary 查询测评列表
// @Description 分页查询测评列表
// @Tags Evaluation-Assessment
// @Produce json
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(10)
// @Param status query string false "状态筛选"
// @Param testee_id query string false "受试者ID筛选"
// @Success 200 {object} core.Response{data=response.AssessmentListResponse}
// @Router /api/v1/evaluations/assessments [get]
func (h *EvaluationHandler) ListAssessments(c *gin.Context) {
	var req request.ListAssessmentsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		h.BadRequestResponse(c, "请求参数无效", err)
		return
	}

	// 设置默认值
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 10
	}

	orgID := h.getOrgIDFromContext(c)
	conditions := make(map[string]string)
	if req.Status != "" {
		conditions["status"] = req.Status
	}
	if req.TesteeID > 0 {
		conditions["testee_id"] = strconv.FormatUint(req.TesteeID, 10)
	}

	dto := assessmentApp.ListAssessmentsDTO{
		OrgID:      orgID,
		Page:       req.Page,
		PageSize:   req.PageSize,
		Conditions: conditions,
	}

	ctx := context.Background()
	result, err := h.managementService.List(ctx, dto)
	if err != nil {
		h.Error(c, err)
		return
	}

	h.Success(c, response.NewAssessmentListResponse(result))
}

// GetStatistics 获取统计数据
// @Summary 获取测评统计数据
// @Description 获取指定时间范围内的测评统计数据
// @Tags Evaluation-Assessment
// @Produce json
// @Param start_time query string false "开始时间（格式：2006-01-02）"
// @Param end_time query string false "结束时间（格式：2006-01-02）"
// @Param scale_code query string false "量表编码筛选"
// @Success 200 {object} core.Response{data=response.AssessmentStatisticsResponse}
// @Router /api/v1/evaluations/assessments/statistics [get]
func (h *EvaluationHandler) GetStatistics(c *gin.Context) {
	var req request.GetStatisticsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		h.BadRequestResponse(c, "请求参数无效", err)
		return
	}

	orgID := h.getOrgIDFromContext(c)
	dto := assessmentApp.GetStatisticsDTO{
		OrgID:     orgID,
		ScaleCode: req.ScaleCode,
	}

	// 解析时间
	if req.StartTime != nil && *req.StartTime != "" {
		t, err := time.Parse("2006-01-02", *req.StartTime)
		if err == nil {
			dto.StartTime = &t
		}
	}
	if req.EndTime != nil && *req.EndTime != "" {
		t, err := time.Parse("2006-01-02", *req.EndTime)
		if err == nil {
			dto.EndTime = &t
		}
	}

	ctx := context.Background()
	result, err := h.managementService.GetStatistics(ctx, dto)
	if err != nil {
		h.Error(c, err)
		return
	}

	h.Success(c, response.NewAssessmentStatisticsResponse(result))
}

// ============= Score 相关接口 =============

// GetScores 获取测评得分
// @Summary 获取测评得分
// @Description 获取指定测评的得分详情。响应中的 factor_scores 包含每个因子的得分信息，其中 max_score 为因子的最大分（可选）
// @Tags Evaluation-Score
// @Produce json
// @Param id path string true "测评ID"
// @Success 200 {object} core.Response{data=response.ScoreResponse}
// @Router /api/v1/evaluations/assessments/{id}/scores [get]
func (h *EvaluationHandler) GetScores(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		h.BadRequestResponse(c, "无效的测评ID", err)
		return
	}

	ctx := context.Background()
	result, err := h.scoreQueryService.GetByAssessmentID(ctx, id)
	if err != nil {
		h.Error(c, err)
		return
	}

	h.Success(c, response.NewScoreResponse(result))
}

// GetFactorTrend 获取因子趋势
// @Summary 获取因子趋势
// @Description 获取指定受试者某因子的历史得分趋势
// @Tags Evaluation-Score
// @Produce json
// @Param testee_id query string true "受试者ID"
// @Param factor_code query string true "因子编码"
// @Param limit query int false "返回记录数限制" default(10)
// @Success 200 {object} core.Response{data=response.FactorTrendResponse}
// @Router /api/v1/evaluations/scores/trend [get]
func (h *EvaluationHandler) GetFactorTrend(c *gin.Context) {
	var req request.GetFactorTrendRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		h.BadRequestResponse(c, "请求参数无效", err)
		return
	}

	if req.Limit <= 0 {
		req.Limit = 10
	}

	dto := assessmentApp.GetFactorTrendDTO{
		TesteeID:   req.TesteeID,
		FactorCode: req.FactorCode,
		Limit:      req.Limit,
	}

	ctx := context.Background()
	result, err := h.scoreQueryService.GetFactorTrend(ctx, dto)
	if err != nil {
		h.Error(c, err)
		return
	}

	h.Success(c, response.NewFactorTrendResponse(result))
}

// GetHighRiskFactors 获取高风险因子
// @Summary 获取高风险因子
// @Description 获取指定测评的高风险因子列表
// @Tags Evaluation-Score
// @Produce json
// @Param id path string true "测评ID"
// @Success 200 {object} core.Response{data=response.HighRiskFactorsResponse}
// @Router /api/v1/evaluations/assessments/{id}/high-risk-factors [get]
func (h *EvaluationHandler) GetHighRiskFactors(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		h.BadRequestResponse(c, "无效的测评ID", err)
		return
	}

	ctx := context.Background()
	result, err := h.scoreQueryService.GetHighRiskFactors(ctx, id)
	if err != nil {
		h.Error(c, err)
		return
	}

	h.Success(c, response.NewHighRiskFactorsResponse(result))
}

// ============= Report 相关接口 =============

// GetReport 获取测评报告
// @Summary 获取测评报告
// @Description 获取指定测评的解读报告。响应字段说明：
// @Description - dimensions（维度列表）：每个维度包含 factor_code（因子编码）、factor_name（因子名称）、raw_score（原始分）、max_score（最大分，可选）、risk_level（风险等级）、description（解读描述）、suggestion（维度建议，字符串）字段
// @Description - suggestions（建议列表）：报告级别的建议列表，每个建议包含 category（分类）、content（内容）、factor_code（关联因子编码，可选）字段
// @Tags Evaluation-Report
// @Produce json
// @Param id path string true "测评ID"
// @Success 200 {object} core.Response{data=response.ReportResponse}
// @Router /api/v1/evaluations/assessments/{id}/report [get]
func (h *EvaluationHandler) GetReport(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		h.BadRequestResponse(c, "无效的测评ID", err)
		return
	}

	ctx := context.Background()
	result, err := h.reportQueryService.GetByAssessmentID(ctx, id)
	if err != nil {
		h.Error(c, err)
		return
	}

	h.Success(c, response.NewReportResponse(result))
}

// ListReports 查询报告列表
// @Summary 查询报告列表
// @Description 查询指定受试者的报告列表。每个报告包含 dimensions（维度列表）和 suggestions（建议列表），维度中的 suggestion 字段为字符串类型
// @Tags Evaluation-Report
// @Produce json
// @Param testee_id query string true "受试者ID"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(10)
// @Success 200 {object} core.Response{data=response.ReportListResponse}
// @Router /api/v1/evaluations/reports [get]
func (h *EvaluationHandler) ListReports(c *gin.Context) {
	var req request.ListReportsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		h.BadRequestResponse(c, "请求参数无效", err)
		return
	}

	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 10
	}

	dto := assessmentApp.ListReportsDTO{
		TesteeID: req.TesteeID,
		Page:     req.Page,
		PageSize: req.PageSize,
	}

	ctx := context.Background()
	result, err := h.reportQueryService.ListByTesteeID(ctx, dto)
	if err != nil {
		h.Error(c, err)
		return
	}

	h.Success(c, response.NewReportListResponse(result))
}

// ============= Evaluation 相关接口（内部/管理员）=============

// BatchEvaluate 批量评估
// @Summary 批量评估
// @Description 批量执行测评评估（管理员/Worker使用）
// @Tags Evaluation-Admin
// @Accept json
// @Produce json
// @Param request body request.BatchEvaluateRequest true "批量评估请求"
// @Success 200 {object} core.Response{data=response.BatchEvaluationResponse}
// @Router /api/v1/evaluations/batch-evaluate [post]
func (h *EvaluationHandler) BatchEvaluate(c *gin.Context) {
	var req request.BatchEvaluateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.BadRequestResponse(c, "请求参数无效", err)
		return
	}

	ctx := context.Background()
	result, err := h.evaluationService.EvaluateBatch(ctx, req.AssessmentIDs)
	if err != nil {
		h.Error(c, err)
		return
	}

	h.Success(c, response.NewBatchEvaluationResponse(result))
}

// RetryFailed 重试失败的测评
// @Summary 重试失败的测评
// @Description 重试指定测评的评估流程
// @Tags Evaluation-Admin
// @Produce json
// @Param id path string true "测评ID"
// @Success 200 {object} core.Response{data=response.AssessmentResponse}
// @Router /api/v1/evaluations/assessments/{id}/retry [post]
func (h *EvaluationHandler) RetryFailed(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		h.BadRequestResponse(c, "无效的测评ID", err)
		return
	}

	ctx := context.Background()
	result, err := h.managementService.Retry(ctx, id)
	if err != nil {
		h.Error(c, err)
		return
	}

	h.Success(c, response.NewAssessmentResponse(result))
}

// ============= 辅助方法 =============

// getOrgIDFromContext 从上下文获取组织ID
// 优先从 JWT claims 的 TenantID 获取，降级到 Header/Query
func (h *EvaluationHandler) getOrgIDFromContext(c *gin.Context) uint64 {
	// 优先从 JWT claims 获取（由 UserIdentityMiddleware 设置）
	orgID := h.GetOrgID(c)
	if orgID > 0 {
		return orgID
	}

	// 降级：从 header 或 query 中获取（兼容旧代码）
	orgIDStr := c.GetHeader("X-Org-ID")
	if orgIDStr == "" {
		orgIDStr = c.Query("org_id")
	}
	if orgIDStr == "" {
		return 0
	}

	orgID, _ = strconv.ParseUint(orgIDStr, 10, 64)
	return orgID
}
