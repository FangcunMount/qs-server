package handler

import (
	"context"
	"errors"
	"strconv"
	"time"

	behaviorassessment "github.com/FangcunMount/qs-server/internal/collection-server/application/behaviorassessment"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/reportwait"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type behaviorAssessmentQueryService interface {
	List(ctx context.Context, testeeID uint64, req *behaviorassessment.ListAssessmentsRequest) (*behaviorassessment.ListAssessmentsResponse, error)
	Get(ctx context.Context, testeeID, assessmentID uint64) (*behaviorassessment.AssessmentDetailResponse, error)
	GetReport(ctx context.Context, testeeID, assessmentID uint64) (*behaviorassessment.AssessmentReportResponse, error)
	GetReportStatus(ctx context.Context, testeeID, assessmentID uint64) (*behaviorassessment.AssessmentStatusResponse, error)
	WaitReport(ctx context.Context, testeeID, assessmentID uint64, timeout time.Duration) (*behaviorassessment.AssessmentStatusResponse, error)
}

type BehaviorAssessmentHandler struct {
	*BaseHandler
	queryService      behaviorAssessmentQueryService
	waitReportService *reportwait.Service
}

func NewBehaviorAssessmentHandler(queryService behaviorAssessmentQueryService, waitReportService *reportwait.Service) *BehaviorAssessmentHandler {
	return &BehaviorAssessmentHandler{BaseHandler: NewBaseHandler(), queryService: queryService, waitReportService: waitReportService}
}

// List lists behavior ability assessments for a testee.
// @Summary 查询行为能力测评列表
// @Description 聚合行为评分（behavioral_rating）与认知（cognitive）测评；不包含医学量表或类型学测评。
// @Tags 行为能力测评
// @Produce json
// @Param testee_id query string true "受试者ID（建议字符串）"
// @Param status query string false "测评状态"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(10)
// @Success 200 {object} core.Response{data=behaviorassessment.ListAssessmentsResponse}
// @Router /api/v1/behavior-assessments [get]
func (h *BehaviorAssessmentHandler) List(c *gin.Context) {
	testeeID, ok := h.parseTesteeID(c)
	if !ok {
		return
	}
	var req behaviorassessment.ListAssessmentsRequest
	if err := h.BindQuery(c, &req); err != nil {
		return
	}
	result, err := h.queryService.List(c.Request.Context(), testeeID, &req)
	if err != nil {
		h.InternalErrorResponse(c, "list behavior assessments failed", err)
		return
	}
	h.Success(c, result)
}

// Get returns a behavior ability assessment detail.
// @Summary 获取行为能力测评详情
// @Tags 行为能力测评
// @Produce json
// @Param id path int true "测评ID"
// @Param testee_id query string true "受试者ID（建议字符串）"
// @Success 200 {object} core.Response{data=behaviorassessment.AssessmentDetailResponse}
// @Router /api/v1/behavior-assessments/{id} [get]
func (h *BehaviorAssessmentHandler) Get(c *gin.Context) {
	testeeID, assessmentID, ok := h.parseRequestIDs(c)
	if !ok {
		return
	}
	result, err := h.queryService.Get(c.Request.Context(), testeeID, assessmentID)
	if err != nil {
		if errors.Is(err, behaviorassessment.ErrNotBehaviorAssessment) {
			h.NotFoundResponse(c, "behavior assessment not found", nil)
			return
		}
		h.InternalErrorResponse(c, "get behavior assessment failed", err)
		return
	}
	if result == nil {
		h.NotFoundResponse(c, "behavior assessment not found", nil)
		return
	}
	h.Success(c, result)
}

// GetReport returns a behavior ability assessment report.
// @Summary 获取行为能力测评报告
// @Description 仅在 report-status 终态 interpreted 后调用。
// @Tags 行为能力测评
// @Produce json
// @Param id path int true "测评ID"
// @Param testee_id query string true "受试者ID（建议字符串）"
// @Success 200 {object} core.Response{data=behaviorassessment.AssessmentReportResponse}
// @Router /api/v1/behavior-assessments/{id}/report [get]
func (h *BehaviorAssessmentHandler) GetReport(c *gin.Context) {
	testeeID, assessmentID, ok := h.parseRequestIDs(c)
	if !ok {
		return
	}
	result, err := h.queryService.GetReport(c.Request.Context(), testeeID, assessmentID)
	if err != nil {
		if errors.Is(err, behaviorassessment.ErrNotBehaviorAssessment) || status.Code(err) == codes.PermissionDenied {
			h.NotFoundResponse(c, "behavior assessment report not found", nil)
			return
		}
		h.InternalErrorResponse(c, "get behavior assessment report failed", err)
		return
	}
	if result == nil {
		h.NotFoundResponse(c, "behavior assessment report not found", nil)
		return
	}
	h.Success(c, result)
}

// GetReportStatus queries behavior ability report status without blocking.
// @Summary 查询行为能力测评报告状态
// @Description 非终态按 next_poll_after_ms 退避；也可使用 WSS /report-events（subscribe.kind=behavior）。
// @Tags 行为能力测评
// @Produce json
// @Param id path int true "测评ID"
// @Param testee_id query string true "受试者ID（建议字符串）"
// @Success 200 {object} core.Response{data=behaviorassessment.AssessmentStatusResponse}
// @Router /api/v1/behavior-assessments/{id}/report-status [get]
func (h *BehaviorAssessmentHandler) GetReportStatus(c *gin.Context) {
	testeeID, assessmentID, ok := h.parseRequestIDs(c)
	if !ok {
		return
	}
	result, err := h.queryService.GetReportStatus(c.Request.Context(), testeeID, assessmentID)
	if err != nil {
		if errors.Is(err, behaviorassessment.ErrNotBehaviorAssessment) {
			h.NotFoundResponse(c, "behavior assessment not found", nil)
			return
		}
		h.InternalErrorResponse(c, "get behavior assessment report status failed", err)
		return
	}
	h.Success(c, result)
}

// WaitReport waits for a behavior ability report. New clients should prefer report-status or WSS.
// @Summary 长轮询等待行为能力测评报告
// @Tags 行为能力测评
// @Produce json
// @Param id path int true "测评ID"
// @Param testee_id query string true "受试者ID（建议字符串）"
// @Param timeout query int false "超时时间（秒）" default(20)
// @Success 200 {object} core.Response{data=behaviorassessment.AssessmentStatusResponse}
// @Router /api/v1/behavior-assessments/{id}/wait-report [get]
func (h *BehaviorAssessmentHandler) WaitReport(c *gin.Context) {
	testeeID, assessmentID, ok := h.parseRequestIDs(c)
	if !ok {
		return
	}
	if h.waitReportService == nil {
		h.InternalErrorResponse(c, "wait report service is not configured", nil)
		return
	}
	result, err := h.queryService.WaitReport(c.Request.Context(), testeeID, assessmentID, h.waitReportService.NormalizeTimeout(c.DefaultQuery("timeout", "20")))
	if err != nil {
		if errors.Is(err, behaviorassessment.ErrNotBehaviorAssessment) {
			h.NotFoundResponse(c, "behavior assessment not found", nil)
			return
		}
		h.InternalErrorResponse(c, "wait behavior assessment report failed", err)
		return
	}
	h.Success(c, result)
}

func (h *BehaviorAssessmentHandler) parseRequestIDs(c *gin.Context) (uint64, uint64, bool) {
	testeeID, ok := h.parseTesteeID(c)
	if !ok {
		return 0, 0, false
	}
	assessmentID, err := strconv.ParseUint(h.GetPathParam(c, "id"), 10, 64)
	if err != nil {
		h.BadRequestResponse(c, "invalid assessment id", err)
		return 0, 0, false
	}
	return testeeID, assessmentID, true
}

func (h *BehaviorAssessmentHandler) parseTesteeID(c *gin.Context) (uint64, bool) {
	raw := h.GetQueryParam(c, "testee_id")
	if raw == "" {
		h.BadRequestResponse(c, "testee_id is required", nil)
		return 0, false
	}
	testeeID, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		h.BadRequestResponse(c, "invalid testee_id format", err)
		return 0, false
	}
	return testeeID, true
}
