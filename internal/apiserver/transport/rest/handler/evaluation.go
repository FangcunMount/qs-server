package handler

import (
	"context"
	"strconv"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/gin-gonic/gin"

	evaluationoperator "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/operator"
	reportqueryjourney "github.com/FangcunMount/qs-server/internal/apiserver/application/journey/reportquery"
	reportwaitjourney "github.com/FangcunMount/qs-server/internal/apiserver/application/journey/reportwait"
	systemgov "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/request"
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/response"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	pkgmiddleware "github.com/FangcunMount/qs-server/internal/pkg/middleware"
)

// EvaluationOperatorHandler exposes backend-operator Evaluation use cases.
type EvaluationOperatorHandler struct {
	*BaseHandler
	operatorExecutionService evaluationoperator.BatchExecutionService
	protectedQueryService    evaluationoperator.QueryService
	systemGovernance         GovernanceActionRunner
}

type GovernanceActionRunner interface {
	RunAction(context.Context, int64, string, systemgov.ActionRunRequest) (*systemgov.ActionRunResult, error)
}

// AssessmentReportJourneyHandler exposes cross-module Assessment and Report journeys.
type AssessmentReportJourneyHandler struct {
	*BaseHandler
	reportQueryJourney reportqueryjourney.Service
	reportWaitJourney  reportwaitjourney.Service
}

func NewEvaluationOperatorHandler(
	operatorExecutionService evaluationoperator.BatchExecutionService,
	protectedQueryService evaluationoperator.QueryService,
	governance ...GovernanceActionRunner,
) *EvaluationOperatorHandler {
	handler := &EvaluationOperatorHandler{BaseHandler: &BaseHandler{}, operatorExecutionService: operatorExecutionService, protectedQueryService: protectedQueryService}
	if len(governance) > 0 {
		handler.systemGovernance = governance[0]
	}
	return handler
}

func NewAssessmentReportJourneyHandler(
	reportQueryJourney reportqueryjourney.Service,
	reportWaitJourney reportwaitjourney.Service,
) *AssessmentReportJourneyHandler {
	return &AssessmentReportJourneyHandler{BaseHandler: &BaseHandler{}, reportQueryJourney: reportQueryJourney, reportWaitJourney: reportWaitJourney}
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
func (h *AssessmentReportJourneyHandler) GetAssessment(c *gin.Context) {
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

	if h.reportQueryJourney != nil {
		projected, projectErr := h.reportQueryJourney.GetAssessmentProjection(c.Request.Context(), reportqueryjourney.Scope{OrgID: orgID, OperatorUserID: operatorUserID}, id)
		err = projectErr
		if err != nil {
			h.Error(c, err)
			return
		}
		h.Success(c, response.NewProjectedAssessmentResponse(projected))
		return
	}
	h.Error(c, errors.WithCode(code.ErrModuleInitializationFailed, "Assessment report journey is not configured"))
}

// ListAssessmentRuns lists evaluation runs for one assessment.
// @Summary 查询测评执行运行列表
// @Description 按 attempt 倒序返回测评执行运行记录
// @Tags Evaluation-Assessment
// @Produce json
// @Param id path string true "测评ID"
// @Param limit query int false "返回条数" default(20)
// @Success 200 {object} core.Response{data=response.EvaluationRunListResponse}
// @Router /api/v1/evaluations/assessments/{id}/runs [get]
func (h *EvaluationOperatorHandler) ListAssessmentRuns(c *gin.Context) {
	assessmentID, scope, err := h.parseProtectedAssessmentQuery(c)
	if err != nil {
		h.BadRequestResponse(c, "无效的测评ID", err)
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	result, err := h.protectedQueryService.ListAssessmentRuns(c.Request.Context(), scope, assessmentID, limit)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, response.NewEvaluationRunListResponse(result))
}

// GetLatestAssessmentRun returns the latest evaluation run for one assessment.
// @Summary 查询测评最新执行运行
// @Description 返回测评最新一次执行运行记录
// @Tags Evaluation-Assessment
// @Produce json
// @Param id path string true "测评ID"
// @Success 200 {object} core.Response{data=response.EvaluationRunResponse}
// @Router /api/v1/evaluations/assessments/{id}/runs/latest [get]
func (h *EvaluationOperatorHandler) GetLatestAssessmentRun(c *gin.Context) {
	assessmentID, scope, err := h.parseProtectedAssessmentQuery(c)
	if err != nil {
		h.BadRequestResponse(c, "无效的测评ID", err)
		return
	}
	result, err := h.protectedQueryService.GetLatestAssessmentRun(c.Request.Context(), scope, assessmentID)
	if err != nil {
		h.Error(c, err)
		return
	}
	if result == nil {
		h.Success(c, nil)
		return
	}
	h.Success(c, response.NewEvaluationRunResponse(result))
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
func (h *AssessmentReportJourneyHandler) ListAssessments(c *gin.Context) {
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
	dto := evaluationoperator.ListQuery{
		Page:     req.Page,
		PageSize: req.PageSize,
		TesteeID: testeeID,
		Status:   req.Status,
	}

	if h.reportQueryJourney != nil {
		projected, projectErr := h.reportQueryJourney.ListAssessmentProjection(c.Request.Context(), reportqueryjourney.Scope{OrgID: orgID, OperatorUserID: operatorUserID}, dto)
		err = projectErr
		if err != nil {
			h.Error(c, err)
			return
		}
		h.Success(c, response.NewProjectedAssessmentListResponse(projected))
		return
	}
	h.Error(c, errors.WithCode(code.ErrModuleInitializationFailed, "Assessment report journey is not configured"))
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
func (h *EvaluationOperatorHandler) GetScores(c *gin.Context) {
	id, scope, err := h.parseProtectedAssessmentQuery(c)
	if err != nil {
		h.Error(c, err)
		return
	}

	result, err := h.protectedQueryService.GetScores(c.Request.Context(), scope, id)
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
func (h *EvaluationOperatorHandler) GetFactorTrend(c *gin.Context) {
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

	dto := evaluationoperator.TrendQuery{
		TesteeID:   req.TesteeID,
		FactorCode: req.FactorCode,
		Limit:      req.Limit,
	}

	result, err := h.protectedQueryService.GetFactorTrend(c.Request.Context(), protectedScope(orgID, operatorUserID), dto)
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
func (h *EvaluationOperatorHandler) GetHighRiskFactors(c *gin.Context) {
	id, scope, err := h.parseProtectedAssessmentQuery(c)
	if err != nil {
		h.Error(c, err)
		return
	}

	result, err := h.protectedQueryService.GetHighRiskFactors(c.Request.Context(), scope, id)
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
func (h *AssessmentReportJourneyHandler) GetReport(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		h.Error(c, err)
		return
	}
	orgID, operatorUserID, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	result, err := h.reportQueryJourney.GetReport(c.Request.Context(), reportqueryjourney.Scope{OrgID: orgID, OperatorUserID: operatorUserID}, id)
	if err != nil {
		h.Error(c, err)
		return
	}

	h.Success(c, response.NewReportResponse(result))
}

// ListReports 查询报告列表
// @Summary 查询报告列表
// @Description 查询当前机构或指定受试者的报告列表。每个报告包含 dimensions（维度列表）和 suggestions（建议列表）
// @Tags Evaluation-Report
// @Produce json
// @Param testee_id query string false "受试者ID；管理员省略时查询当前机构"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(10)
// @Success 200 {object} core.Response{data=response.ReportListResponse}
// @Failure 429 {object} core.ErrResponse
// @Router /api/v1/evaluations/reports [get]
func (h *AssessmentReportJourneyHandler) ListReports(c *gin.Context) {
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

	result, err := h.reportQueryJourney.ListReports(c.Request.Context(), reportqueryjourney.Scope{OrgID: orgID, OperatorUserID: operatorUserID}, dto)
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
func (h *EvaluationOperatorHandler) BatchEvaluate(c *gin.Context) {
	var req request.BatchEvaluateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.BadRequestResponse(c, "请求参数无效", err)
		return
	}
	orgID, operatorUserID, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return
	}

	ctx := c.Request.Context()
	if h.operatorExecutionService == nil {
		h.Error(c, errors.WithCode(code.ErrModuleInitializationFailed, "评估引擎服务未初始化"))
		return
	}
	result, err := h.operatorExecutionService.EvaluateBatch(ctx, evaluationoperator.Actor{OrgID: orgID, OperatorUserID: operatorUserID}, req.AssessmentIDs)
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
// @Deprecated
// @Router /api/v1/evaluations/assessments/{id}/retry [post]
func (h *EvaluationOperatorHandler) RetryFailed(c *gin.Context) {
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
	if h.systemGovernance == nil || h.protectedQueryService == nil {
		h.Error(c, errors.WithCode(code.ErrModuleInitializationFailed, "evaluation retry governance is not configured"))
		return
	}
	actor := evaluationoperator.Actor{OrgID: orgID, OperatorUserID: operatorUserID}
	latest, err := h.protectedQueryService.GetLatestAssessmentRun(ctx, actor, id)
	if err != nil {
		h.Error(c, err)
		return
	}
	requestID := pkgmiddleware.RequestIDFromStandardContext(ctx)
	isAuditReplay := latest != nil && requestID != "" && latest.ActionRequestID == requestID
	if latest == nil || (!isAuditReplay && (latest.Status != "failed" || latest.RetryDisposition != "manual_required")) {
		h.Error(c, errors.WithCode(code.ErrConflict, "最新失败尝试不需要人工重试"))
		return
	}
	_, err = h.systemGovernance.RunAction(ctx, orgID, "evaluation.retry", systemgov.ActionRunRequest{
		RequestID: requestID,
		Confirm:   true,
		Input: map[string]interface{}{
			"resource_id":      strconv.FormatUint(id, 10),
			"expected_attempt": latest.AttemptNo,
			"reason":           "legacy evaluation retry endpoint",
		},
	})
	if err != nil {
		h.Error(c, err)
		return
	}
	result, err := h.protectedQueryService.GetAssessment(ctx, actor, id)
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
// @Success 200 {object} core.Response
// @Failure 429 {object} core.ErrResponse
// @Router /api/v1/assessments/{id}/wait-report [get]
func (h *AssessmentReportJourneyHandler) WaitReport(c *gin.Context) {
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

	ctx, cancel := context.WithTimeout(c.Request.Context(), parseWaitReportTimeout(c.DefaultQuery("timeout", "15")))
	defer cancel()

	summary, err := h.reportWaitJourney.Wait(ctx, reportwaitjourney.Scope{
		OrgID:          orgID,
		OperatorUserID: operatorUserID,
		AssessmentID:   id,
	})
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, summary)
}

// ============= 辅助方法 =============

func (h *EvaluationOperatorHandler) parseAssessmentID(c *gin.Context) (uint64, error) {
	return strconv.ParseUint(c.Param("id"), 10, 64)
}

func (h *EvaluationOperatorHandler) parseProtectedAssessmentQuery(c *gin.Context) (uint64, evaluationoperator.Actor, error) {
	id, err := h.parseAssessmentID(c)
	if err != nil {
		return 0, evaluationoperator.Actor{}, err
	}
	orgID, operatorUserID, err := h.RequireProtectedScope(c)
	if err != nil {
		return 0, evaluationoperator.Actor{}, err
	}
	return id, protectedScope(orgID, operatorUserID), nil
}

func protectedScope(orgID, operatorUserID int64) evaluationoperator.Actor {
	return evaluationoperator.Actor{
		OrgID:          orgID,
		OperatorUserID: operatorUserID,
	}
}

func parseWaitReportTimeout(raw string) time.Duration {
	timeoutSeconds, err := strconv.Atoi(raw)
	if err != nil || timeoutSeconds < 5 || timeoutSeconds > 60 {
		timeoutSeconds = 15
	}
	return time.Duration(timeoutSeconds) * time.Second
}
