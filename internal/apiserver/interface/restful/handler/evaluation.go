package handler

import (
	"context"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	actorAccessApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/access"
	assessmentApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/engine"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/waiter"
	"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/request"
	"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/response"
)

// EvaluationHandler 评估模块 Handler
// 提供测评管理、得分查询、报告查询等 RESTful API
type EvaluationHandler struct {
	*BaseHandler
	managementService   assessmentApp.AssessmentManagementService
	reportQueryService  assessmentApp.ReportQueryService
	scoreQueryService   assessmentApp.ScoreQueryService
	evaluationService   engine.Service
	testeeAccessService actorAccessApp.TesteeAccessService
	waiterRegistry      *waiter.WaiterRegistry
}

type accessibleAssessmentContext struct {
	ctx          context.Context
	assessmentID uint64
	assessment   *assessmentApp.AssessmentResult
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

// SetTesteeAccessService 设置 testee 访问控制服务。
func (h *EvaluationHandler) SetTesteeAccessService(testeeAccessService actorAccessApp.TesteeAccessService) {
	h.testeeAccessService = testeeAccessService
}

// SetWaiterRegistry 设置等待队列注册表（可选）
func (h *EvaluationHandler) SetWaiterRegistry(waiterRegistry *waiter.WaiterRegistry) {
	h.waiterRegistry = waiterRegistry
}

// ============= Assessment 查询接口（后台管理）=============

// GetAssessment 获取测评详情
// @Summary 获取测评详情
// @Description 根据ID获取测评详细信息
// @Tags Evaluation-Assessment
// @Produce json
// @Param id path string true "测评ID"
// @Success 200 {object} core.Response{data=response.AssessmentResponse}
// @Failure 429 {object} core.ErrResponse
// @Router /api/v1/evaluations/assessments/{id} [get]
func (h *EvaluationHandler) GetAssessment(c *gin.Context) {
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

	ctx := c.Request.Context()
	result, err := h.managementService.GetByID(ctx, id)
	if err != nil {
		h.Error(c, err)
		return
	}
	if err := h.testeeAccessService.ValidateTesteeAccess(ctx, orgID, operatorUserID, result.TesteeID); err != nil {
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
// @Failure 429 {object} core.ErrResponse
// @Router /api/v1/evaluations/assessments [get]
func (h *EvaluationHandler) ListAssessments(c *gin.Context) {
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

	// 设置默认值
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 10
	}

	conditions := make(map[string]string)
	if req.Status != "" {
		conditions["status"] = req.Status
	}
	if req.TesteeID > 0 {
		if err := h.testeeAccessService.ValidateTesteeAccess(c.Request.Context(), orgID, operatorUserID, req.TesteeID); err != nil {
			h.Error(c, err)
			return
		}
		conditions["testee_id"] = strconv.FormatUint(req.TesteeID, 10)
	}

	dto := assessmentApp.ListAssessmentsDTO{
		OrgID:      uint64(orgID),
		Page:       req.Page,
		PageSize:   req.PageSize,
		Conditions: conditions,
	}

	scope, err := h.testeeAccessService.ResolveAccessScope(c.Request.Context(), orgID, operatorUserID)
	if err != nil {
		h.Error(c, err)
		return
	}
	if !scope.IsAdmin && req.TesteeID == 0 {
		allowedTesteeIDs, err := h.testeeAccessService.ListAccessibleTesteeIDs(c.Request.Context(), orgID, operatorUserID)
		if err != nil {
			h.Error(c, err)
			return
		}
		dto.AccessibleTesteeIDs = allowedTesteeIDs
		dto.RestrictToAccessScope = true
	}

	ctx := c.Request.Context()
	result, err := h.managementService.List(ctx, dto)
	if err != nil {
		h.Error(c, err)
		return
	}

	h.Success(c, response.NewAssessmentListResponse(result))
}

// ============= Score 相关接口 =============

// GetScores 获取测评得分
// @Summary 获取测评得分
// @Description 获取指定测评的得分详情。响应中的 factor_scores 包含每个因子的得分信息，其中 max_score 为因子的最大分（可选）
// @Tags Evaluation-Score
// @Produce json
// @Param id path string true "测评ID"
// @Success 200 {object} core.Response{data=response.ScoreResponse}
// @Failure 429 {object} core.ErrResponse
// @Router /api/v1/evaluations/assessments/{id}/scores [get]
func (h *EvaluationHandler) GetScores(c *gin.Context) {
	assessmentCtx, err := h.loadAccessibleAssessmentContext(c)
	if err != nil {
		h.Error(c, err)
		return
	}

	result, err := h.scoreQueryService.GetByAssessmentID(assessmentCtx.ctx, assessmentCtx.assessmentID)
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
// @Failure 429 {object} core.ErrResponse
// @Router /api/v1/evaluations/scores/trend [get]
func (h *EvaluationHandler) GetFactorTrend(c *gin.Context) {
	orgID, operatorUserID, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return
	}

	var req request.GetFactorTrendRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		h.BadRequestResponse(c, "请求参数无效", err)
		return
	}

	if req.Limit <= 0 {
		req.Limit = 10
	}
	if err := h.testeeAccessService.ValidateTesteeAccess(c.Request.Context(), orgID, operatorUserID, req.TesteeID); err != nil {
		h.Error(c, err)
		return
	}

	dto := assessmentApp.GetFactorTrendDTO{
		TesteeID:   req.TesteeID,
		FactorCode: req.FactorCode,
		Limit:      req.Limit,
	}

	ctx := c.Request.Context()
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
// @Failure 429 {object} core.ErrResponse
// @Router /api/v1/evaluations/assessments/{id}/high-risk-factors [get]
func (h *EvaluationHandler) GetHighRiskFactors(c *gin.Context) {
	assessmentCtx, err := h.loadAccessibleAssessmentContext(c)
	if err != nil {
		h.Error(c, err)
		return
	}

	result, err := h.scoreQueryService.GetHighRiskFactors(assessmentCtx.ctx, assessmentCtx.assessmentID)
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
// @Failure 429 {object} core.ErrResponse
// @Router /api/v1/evaluations/assessments/{id}/report [get]
func (h *EvaluationHandler) GetReport(c *gin.Context) {
	assessmentCtx, err := h.loadAccessibleAssessmentContext(c)
	if err != nil {
		h.Error(c, err)
		return
	}

	result, err := h.reportQueryService.GetByAssessmentID(assessmentCtx.ctx, assessmentCtx.assessmentID)
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
// @Failure 429 {object} core.ErrResponse
// @Router /api/v1/evaluations/reports [get]
func (h *EvaluationHandler) ListReports(c *gin.Context) {
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
	if req.TesteeID != 0 {
		if err := h.testeeAccessService.ValidateTesteeAccess(c.Request.Context(), orgID, operatorUserID, req.TesteeID); err != nil {
			h.Error(c, err)
			return
		}
	} else {
		scope, err := h.testeeAccessService.ResolveAccessScope(c.Request.Context(), orgID, operatorUserID)
		if err != nil {
			h.Error(c, err)
			return
		}
		if scope.IsAdmin {
			h.BadRequestResponse(c, "受试者ID不能为空", nil)
			return
		}
		allowedTesteeIDs, err := h.testeeAccessService.ListAccessibleTesteeIDs(c.Request.Context(), orgID, operatorUserID)
		if err != nil {
			h.Error(c, err)
			return
		}
		dto.AccessibleTesteeIDs = allowedTesteeIDs
		dto.RestrictToAccessScope = true
	}

	ctx := c.Request.Context()
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
// @Description 批量执行测评评估；仅 qs:evaluator 或 qs:admin 可访问
// @Tags Evaluation-Admin
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param request body request.BatchEvaluateRequest true "批量评估请求"
// @Success 200 {object} core.Response{data=response.BatchEvaluationResponse}
// @Failure 429 {object} core.ErrResponse
// @Router /api/v1/evaluations/batch-evaluate [post]
func (h *EvaluationHandler) BatchEvaluate(c *gin.Context) {
	var req request.BatchEvaluateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.BadRequestResponse(c, "请求参数无效", err)
		return
	}
	orgID, err := h.RequireProtectedOrgID(c)
	if err != nil {
		h.Error(c, err)
		return
	}

	ctx := c.Request.Context()
	result, err := h.evaluationService.EvaluateBatch(ctx, orgID, req.AssessmentIDs)
	if err != nil {
		h.Error(c, err)
		return
	}

	h.Success(c, response.NewBatchEvaluationResponse(result))
}

// RetryFailed 重试失败的测评
// @Summary 重试失败的测评
// @Description 重试指定测评的评估流程；仅 qs:evaluator 或 qs:admin 可访问
// @Tags Evaluation-Admin
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param id path string true "测评ID"
// @Success 200 {object} core.Response{data=response.AssessmentResponse}
// @Failure 429 {object} core.ErrResponse
// @Router /api/v1/evaluations/assessments/{id}/retry [post]
func (h *EvaluationHandler) RetryFailed(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		h.BadRequestResponse(c, "无效的测评ID", err)
		return
	}
	orgID, err := h.RequireProtectedOrgID(c)
	if err != nil {
		h.Error(c, err)
		return
	}

	ctx := c.Request.Context()
	result, err := h.managementService.Retry(ctx, orgID, id)
	if err != nil {
		h.Error(c, err)
		return
	}

	h.Success(c, response.NewAssessmentResponse(result))
}

// WaitReport 长轮询等待报告生成
// @Summary 长轮询等待报告生成
// @Description 等待测评报告生成，支持长轮询机制。如果报告已生成则立即返回，否则等待最多 timeout 秒
// @Tags Evaluation-Assessment
// @Produce json
// @Param id path string true "测评ID"
// @Param timeout query int false "超时时间（秒）" default(15) minimum(5) maximum(60)
// @Success 200 {object} core.Response{data=waiter.StatusSummary}
// @Failure 429 {object} core.ErrResponse
// @Router /api/v1/assessments/{id}/wait-report [get]
func (h *EvaluationHandler) WaitReport(c *gin.Context) {
	id, err := h.parseAssessmentID(c)
	if err != nil {
		h.BadRequestResponse(c, "无效的测评ID", err)
		return
	}
	orgID, operatorUserID, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), parseWaitReportTimeout(c.DefaultQuery("timeout", "15")))
	defer cancel()

	assessmentCtx, err := h.loadAccessibleAssessment(ctx, id, orgID, operatorUserID)
	if err != nil {
		h.Error(c, err)
		return
	}
	if summary, done := assessmentStatusSummary(assessmentCtx.assessment); done {
		h.Success(c, summary)
		return
	}

	h.Success(c, h.waitForReportSummary(ctx, id))
}

// ============= 辅助方法 =============

func (h *EvaluationHandler) parseAssessmentID(c *gin.Context) (uint64, error) {
	return strconv.ParseUint(c.Param("id"), 10, 64)
}

func (h *EvaluationHandler) loadAccessibleAssessmentContext(c *gin.Context) (*accessibleAssessmentContext, error) {
	id, err := h.parseAssessmentID(c)
	if err != nil {
		return nil, err
	}
	orgID, operatorUserID, err := h.RequireProtectedScope(c)
	if err != nil {
		return nil, err
	}
	return h.loadAccessibleAssessment(c.Request.Context(), id, orgID, operatorUserID)
}

func (h *EvaluationHandler) loadAccessibleAssessment(ctx context.Context, assessmentID uint64, orgID, operatorUserID int64) (*accessibleAssessmentContext, error) {
	assessmentResult, err := h.managementService.GetByID(ctx, assessmentID)
	if err != nil {
		return nil, err
	}
	if err := h.testeeAccessService.ValidateTesteeAccess(ctx, orgID, operatorUserID, assessmentResult.TesteeID); err != nil {
		return nil, err
	}
	return &accessibleAssessmentContext{
		ctx:          ctx,
		assessmentID: assessmentID,
		assessment:   assessmentResult,
	}, nil
}

func parseWaitReportTimeout(raw string) time.Duration {
	timeoutSeconds, err := strconv.Atoi(raw)
	if err != nil || timeoutSeconds < 5 || timeoutSeconds > 60 {
		timeoutSeconds = 15
	}
	return time.Duration(timeoutSeconds) * time.Second
}

func assessmentStatusSummary(result *assessmentApp.AssessmentResult) (waiter.StatusSummary, bool) {
	if result == nil || !isTerminalAssessmentStatus(result.Status) {
		return waiter.StatusSummary{}, false
	}
	return buildAssessmentStatusSummary(result), true
}

func isTerminalAssessmentStatus(status string) bool {
	return status == "interpreted" || status == "failed"
}

func buildAssessmentStatusSummary(result *assessmentApp.AssessmentResult) waiter.StatusSummary {
	var totalScore *float64
	if result.TotalScore != nil {
		value := *result.TotalScore
		totalScore = &value
	}

	var riskLevel *string
	if result.RiskLevel != nil {
		value := *result.RiskLevel
		riskLevel = &value
	}

	return waiter.StatusSummary{
		Status:     result.Status,
		TotalScore: totalScore,
		RiskLevel:  riskLevel,
		UpdatedAt:  time.Now().Unix(),
	}
}

func pendingAssessmentStatusSummary() waiter.StatusSummary {
	return waiter.StatusSummary{
		Status:    "pending",
		UpdatedAt: time.Now().Unix(),
	}
}

func (h *EvaluationHandler) waitForReportSummary(ctx context.Context, assessmentID uint64) waiter.StatusSummary {
	if h.waiterRegistry == nil {
		return h.waitForReportByPolling(ctx, assessmentID)
	}
	return h.waitForReportWithRegistry(ctx, assessmentID)
}

func (h *EvaluationHandler) waitForReportByPolling(ctx context.Context, assessmentID uint64) waiter.StatusSummary {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return pendingAssessmentStatusSummary()
		case <-ticker.C:
			if summary, done := h.loadTerminalAssessmentSummary(ctx, assessmentID); done {
				return summary
			}
		}
	}
}

func (h *EvaluationHandler) waitForReportWithRegistry(ctx context.Context, assessmentID uint64) waiter.StatusSummary {
	ch := make(chan waiter.StatusSummary, 1)
	h.waiterRegistry.Add(assessmentID, ch)
	defer h.waiterRegistry.Remove(assessmentID, ch)

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return pendingAssessmentStatusSummary()
		case summary := <-ch:
			return summary
		case <-ticker.C:
			if summary, done := h.loadTerminalAssessmentSummary(ctx, assessmentID); done {
				return summary
			}
		}
	}
}

func (h *EvaluationHandler) loadTerminalAssessmentSummary(ctx context.Context, assessmentID uint64) (waiter.StatusSummary, bool) {
	result, err := h.managementService.GetByID(ctx, assessmentID)
	if err != nil || result == nil {
		return waiter.StatusSummary{}, false
	}
	return assessmentStatusSummary(result)
}
