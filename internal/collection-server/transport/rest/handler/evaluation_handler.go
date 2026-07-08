package handler

import (
	"context"
	"strconv"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/evaluation"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/reportstatus"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/reportwait"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/ratelimit"
	"github.com/FangcunMount/qs-server/pkg/core"
	"github.com/gin-gonic/gin"
)

type evaluationQueryService interface {
	GetAssessmentScores(ctx context.Context, testeeID, assessmentID uint64) ([]evaluation.FactorScoreResponse, error)
	GetFactorTrend(ctx context.Context, testeeID uint64, req *evaluation.GetFactorTrendRequest) ([]evaluation.TrendPointResponse, error)
	GetAssessmentTrendSummary(ctx context.Context, testeeID, assessmentID uint64) (*evaluation.AssessmentTrendSummaryResponse, error)
	GetHighRiskFactors(ctx context.Context, testeeID, assessmentID uint64) ([]evaluation.FactorScoreResponse, error)
	GetMyAssessment(ctx context.Context, testeeID, assessmentID uint64) (*evaluation.AssessmentDetailResponse, error)
	ListMyAssessments(ctx context.Context, testeeID uint64, req *evaluation.ListAssessmentsRequest) (*evaluation.ListAssessmentsResponse, error)
	GetAssessmentReport(ctx context.Context, testeeID, assessmentID uint64) (*evaluation.AssessmentReportResponse, error)
}

type waitReportService interface {
	NormalizeTimeout(raw string) time.Duration
	GetStatus(ctx context.Context, testeeID, assessmentID uint64) (*evaluation.AssessmentStatusResponse, error)
	Wait(ctx context.Context, testeeID, assessmentID uint64, timeout time.Duration) (*evaluation.AssessmentStatusResponse, error)
}

// EvaluationHandler 测评处理器
type EvaluationHandler struct {
	*BaseHandler
	queryService      evaluationQueryService
	waitReportService waitReportService
}

// NewEvaluationHandler 创建测评处理器
func NewEvaluationHandler(
	queryService evaluationQueryService,
	waitReportService waitReportService,
) *EvaluationHandler {
	if waitReportService == nil {
		waitReportService = reportwait.NewService(queryService, nil, nil, nil, reportwait.DefaultConfig())
	}
	return &EvaluationHandler{
		BaseHandler:       NewBaseHandler(),
		queryService:      queryService,
		waitReportService: waitReportService,
	}
}

// GetAssessmentScores 获取测评得分详情
// @Summary 获取测评得分详情
// @Description 获取测评的因子得分详情。响应中的每个因子得分包含 max_score（最大分，可选）字段
// @Tags 测评
// @Produce json
// @Param id path int true "测评ID"
// @Param testee_id query int true "受试者ID"
// @Success 200 {object} core.Response{data=[]evaluation.FactorScoreResponse}
// @Failure 429 {object} core.ErrResponse
// @Failure 400 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Security Bearer
// @Router /api/v1/assessments/{id}/scores [get]
func (h *EvaluationHandler) GetAssessmentScores(c *gin.Context) {
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
	assessmentID, ok := h.parseRequiredAssessmentID(c)
	if !ok {
		return
	}
	result, err := h.queryService.GetAssessmentScores(c.Request.Context(), testeeID, assessmentID)
	if err != nil {
		h.InternalErrorResponse(c, "get scores failed", err)
		return
	}
	h.Success(c, result)
}

// GetReportStatus 短轮询查询报告生成状态（非阻塞）。
// @Summary 查询报告生成状态
// @Description 立即返回当前报告状态；非终态时通过 next_poll_after_ms 指引客户端退避重试
// @Tags 测评
// @Produce json
// @Param id path int true "测评ID"
// @Param testee_id query int true "受试者ID"
// @Success 200 {object} core.Response{data=evaluation.AssessmentStatusResponse}
// @Failure 429 {object} core.ErrResponse
// @Failure 400 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Security Bearer
// @Router /api/v1/assessments/{id}/report-status [get]
func (h *EvaluationHandler) GetReportStatus(c *gin.Context) {
	testeeID, assessmentID, ok := h.parseReportStatusRequest(c)
	if !ok {
		return
	}
	statusResponse, err := h.waitReportService.GetStatus(c.Request.Context(), testeeID, assessmentID)
	if err != nil {
		h.InternalErrorResponse(c, "get report status failed", err)
		return
	}
	h.Success(c, reportstatus.ToPublicAssessmentStatus(statusResponse))
}

// WaitReport 长轮询等待报告生成
// @Summary 长轮询等待报告生成
// @Description 等待测评报告生成，支持长轮询机制。如果报告已生成则立即返回，否则等待最多 timeout 秒
// @Tags 测评
// @Produce json
// @Param id path int true "测评ID"
// @Param testee_id query int true "受试者ID"
// @Param timeout query int false "超时时间（秒）" default(20) minimum(1) maximum(25)
// @Success 200 {object} core.Response{data=evaluation.AssessmentStatusResponse}
// @Failure 429 {object} core.ErrResponse
// @Failure 400 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Security Bearer
// @Router /api/v1/assessments/{id}/wait-report [get]
func (h *EvaluationHandler) WaitReport(c *gin.Context) {
	testeeID, assessmentID, timeout, ok := h.parseWaitReportRequest(c)
	if !ok {
		return
	}
	start := time.Now()
	ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
	defer cancel()

	statusResponse, err := h.waitReportService.Wait(ctx, testeeID, assessmentID, timeout)
	if err != nil {
		logger.L(c.Request.Context()).Errorw("wait-report failed",
			"assessment_id", assessmentID,
			"testee_id", testeeID,
			"timeout_ms", timeout.Milliseconds(),
			"elapsed_ms", time.Since(start).Milliseconds(),
			"error", err.Error(),
		)
		h.InternalErrorResponse(c, "wait report failed", err)
		return
	}
	logger.L(c.Request.Context()).Infow("wait-report completed",
		"assessment_id", assessmentID,
		"testee_id", testeeID,
		"status", statusResponse.Status,
		"stage", statusResponse.Stage,
		"next_poll_after_ms", statusResponse.NextPollAfterMs,
		"timeout_ms", timeout.Milliseconds(),
		"elapsed_ms", time.Since(start).Milliseconds(),
	)
	publicStatus := reportstatus.ToPublicAssessmentStatus(statusResponse)
	applyReportPollRetryAfter(c, publicStatus)
	h.Success(c, publicStatus)
}

func applyReportPollRetryAfter(c *gin.Context, status *evaluation.AssessmentStatusResponse) {
	if status == nil || status.Status == "interpreted" || status.Status == "completed" || status.Status == "failed" {
		return
	}
	retryAfterSec := (status.NextPollAfterMs + 999) / 1000
	if retryAfterSec <= 0 {
		retryAfterSec = 3
	}
	ratelimit.ApplyRetryAfterSeconds(c.Writer.Header(), retryAfterSec)
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
// @Failure 429 {object} core.ErrResponse
// @Failure 400 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Security Bearer
// @Router /api/v1/assessments/trend [get]
func (h *EvaluationHandler) GetFactorTrend(c *gin.Context) {
	testeeID, ok := h.parseRequiredTesteeID(c)
	if !ok {
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

// GetAssessmentTrendSummary 获取测评趋势摘要
// @Summary 获取测评趋势摘要
// @Description 获取指定测评所属同量表、同版本的历史趋势摘要
// @Tags 测评
// @Produce json
// @Param id path int true "测评ID"
// @Param testee_id query int true "受试者ID"
// @Success 200 {object} core.Response{data=evaluation.AssessmentTrendSummaryResponse}
// @Failure 429 {object} core.ErrResponse
// @Failure 400 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Security Bearer
// @Router /api/v1/assessments/{id}/trend-summary [get]
func (h *EvaluationHandler) GetAssessmentTrendSummary(c *gin.Context) {
	testeeID, ok := h.parseRequiredTesteeID(c)
	if !ok {
		return
	}
	assessmentID, ok := h.parseRequiredAssessmentID(c)
	if !ok {
		return
	}
	result, err := h.queryService.GetAssessmentTrendSummary(c.Request.Context(), testeeID, assessmentID)
	if err != nil {
		h.InternalErrorResponse(c, "get trend summary failed", err)
		return
	}
	if result == nil {
		h.NotFoundResponse(c, "assessment not found", nil)
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
// @Failure 429 {object} core.ErrResponse
// @Failure 400 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Security Bearer
// @Router /api/v1/assessments/{id}/factors/high-risk [get]
func (h *EvaluationHandler) GetHighRiskFactors(c *gin.Context) {
	testeeID, ok := h.parseRequiredTesteeID(c)
	if !ok {
		return
	}
	assessmentID, ok := h.parseRequiredAssessmentID(c)
	if !ok {
		return
	}
	result, err := h.queryService.GetHighRiskFactors(c.Request.Context(), testeeID, assessmentID)
	if err != nil {
		h.InternalErrorResponse(c, "get high risk factors failed", err)
		return
	}
	h.Success(c, result)
}

// GetMyAssessment 获取测评详情（outcome 投影，/api/v2）。
// Deprecated: 请优先使用 /api/v1/typology-assessments。
// @Summary 获取测评详情
// @Description 根据测评 ID 获取详情，响应使用 model/primary_score/level 投影
// @Tags 测评
// @Produce json
// @Param id path int true "测评ID"
// @Param testee_id query int true "受试者ID"
// @Success 200 {object} core.Response{data=evaluation.AssessmentDetailResponse}
// @Router /api/v2/assessments/{id} [get]
func (h *EvaluationHandler) GetMyAssessment(c *gin.Context) {
	testeeID, assessmentID, ok := h.parseTesteeAndAssessmentID(c)
	if !ok {
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

// ListMyAssessments 查询测评列表（outcome 投影，/api/v2）。
// Deprecated: 请优先使用 /api/v1/typology-assessments。
// @Summary 查询测评列表
// @Description 分页查询测评列表，响应使用 model/primary_score/level 投影
// @Tags 测评
// @Produce json
// @Param testee_id query int true "受试者ID"
// @Success 200 {object} core.Response{data=evaluation.ListAssessmentsResponse}
// @Router /api/v2/assessments [get]
func (h *EvaluationHandler) ListMyAssessments(c *gin.Context) {
	testeeID, req, ok := h.bindAssessmentListQuery(c)
	if !ok {
		return
	}
	result, err := h.queryService.ListMyAssessments(c.Request.Context(), testeeID, &req)
	if err != nil {
		respondAssessmentListError(h, c, err)
		return
	}
	h.Success(c, result)
}

// GetAssessmentReport 获取测评报告（outcome 投影，/api/v2）。
// Deprecated: 请优先使用 /api/v1/typology-assessments/{id}/report。
// @Summary 获取测评报告
// @Description 根据测评 ID 获取报告，响应使用 model/primary_score/level 投影。必须传 testee_id 校验归属。
// @Tags 测评
// @Produce json
// @Param id path int true "测评ID"
// @Param testee_id query int true "受试者ID"
// @Success 200 {object} core.Response{data=evaluation.AssessmentReportResponse}
// @Failure 400 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /api/v2/assessments/{id}/report [get]
func (h *EvaluationHandler) GetAssessmentReport(c *gin.Context) {
	testeeID, assessmentID, ok := h.parseTesteeAndAssessmentID(c)
	if !ok {
		return
	}
	result, err := h.queryService.GetAssessmentReport(c.Request.Context(), testeeID, assessmentID)
	if err != nil {
		respondOutcomeAssessmentReportError(h, c, err)
		return
	}
	if result == nil {
		h.NotFoundResponse(c, "report not found", nil)
		return
	}
	h.Success(c, result)
}
