package handler

import (
	"strconv"

	personalityassessment "github.com/FangcunMount/qs-server/internal/collection-server/application/personalityassessment"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/reportwait"
	"github.com/gin-gonic/gin"
)

type PersonalityAssessmentHandler struct {
	*BaseHandler
	queryService      *personalityassessment.QueryService
	waitReportService *reportwait.Service
}

func NewPersonalityAssessmentHandler(
	queryService *personalityassessment.QueryService,
	waitReportService *reportwait.Service,
) *PersonalityAssessmentHandler {
	return &PersonalityAssessmentHandler{
		BaseHandler:       NewBaseHandler(),
		queryService:      queryService,
		waitReportService: waitReportService,
	}
}

// List lists personality assessments for a testee.
// @Summary 查询人格测评列表
// @Tags 人格测评
// @Produce json
// @Param testee_id query int true "受试者ID"
// @Param algorithm query string false "算法过滤 mbti/sbti"
// @Success 200 {object} core.Response{data=personalityassessment.ListAssessmentsResponse}
// @Router /api/v1/personality-assessments [get]
func (h *PersonalityAssessmentHandler) List(c *gin.Context) {
	testeeID, ok := h.parseTesteeID(c)
	if !ok {
		return
	}
	var req personalityassessment.ListAssessmentsRequest
	if err := h.BindQuery(c, &req); err != nil {
		return
	}
	result, err := h.queryService.List(c.Request.Context(), testeeID, &req)
	if err != nil {
		h.InternalErrorResponse(c, "list personality assessments failed", err)
		return
	}
	h.Success(c, result)
}

// Get returns a personality assessment detail.
// @Summary 获取人格测评详情
// @Tags 人格测评
// @Produce json
// @Param id path int true "测评ID"
// @Param testee_id query int true "受试者ID"
// @Success 200 {object} core.Response{data=personalityassessment.AssessmentDetailResponse}
// @Router /api/v1/personality-assessments/{id} [get]
func (h *PersonalityAssessmentHandler) Get(c *gin.Context) {
	testeeID, ok := h.parseTesteeID(c)
	if !ok {
		return
	}
	assessmentID, ok := h.parseAssessmentID(c)
	if !ok {
		return
	}
	result, err := h.queryService.Get(c.Request.Context(), testeeID, assessmentID)
	if err != nil {
		if personalityassessment.IsNotPersonalityAssessment(err) {
			h.NotFoundResponse(c, "personality assessment not found", err)
			return
		}
		h.InternalErrorResponse(c, "get personality assessment failed", err)
		return
	}
	if result == nil {
		h.NotFoundResponse(c, "personality assessment not found", nil)
		return
	}
	h.Success(c, result)
}

// GetReport returns a personality assessment report.
// @Summary 获取人格测评报告
// @Tags 人格测评
// @Produce json
// @Param id path int true "测评ID"
// @Success 200 {object} core.Response{data=personalityassessment.AssessmentReportResponse}
// @Router /api/v1/personality-assessments/{id}/report [get]
func (h *PersonalityAssessmentHandler) GetReport(c *gin.Context) {
	assessmentID, ok := h.parseAssessmentID(c)
	if !ok {
		return
	}
	result, err := h.queryService.GetReport(c.Request.Context(), assessmentID)
	if err != nil {
		if personalityassessment.IsNotPersonalityAssessment(err) {
			h.NotFoundResponse(c, "personality assessment report not found", err)
			return
		}
		h.InternalErrorResponse(c, "get personality assessment report failed", err)
		return
	}
	if result == nil {
		h.NotFoundResponse(c, "personality assessment report not found", nil)
		return
	}
	h.Success(c, result)
}

// WaitReport waits for a personality assessment report.
// @Summary 长轮询等待人格测评报告
// @Tags 人格测评
// @Produce json
// @Param id path int true "测评ID"
// @Param testee_id query int true "受试者ID"
// @Param timeout query int false "超时时间（秒）" default(20)
// @Success 200 {object} core.Response{data=personalityassessment.AssessmentStatusResponse}
// @Router /api/v1/personality-assessments/{id}/wait-report [get]
func (h *PersonalityAssessmentHandler) WaitReport(c *gin.Context) {
	testeeID, ok := h.parseTesteeID(c)
	if !ok {
		return
	}
	assessmentID, ok := h.parseAssessmentID(c)
	if !ok {
		return
	}
	timeout := h.waitReportService.NormalizeTimeout(c.DefaultQuery("timeout", "20"))
	result, err := h.queryService.WaitReport(c.Request.Context(), testeeID, assessmentID, timeout)
	if err != nil {
		h.InternalErrorResponse(c, "wait personality assessment report failed", err)
		return
	}
	h.Success(c, result)
}

func (h *PersonalityAssessmentHandler) parseTesteeID(c *gin.Context) (uint64, bool) {
	testeeIDStr := h.GetQueryParam(c, "testee_id")
	if testeeIDStr == "" {
		h.BadRequestResponse(c, "testee_id is required", nil)
		return 0, false
	}
	testeeID, err := strconv.ParseUint(testeeIDStr, 10, 64)
	if err != nil {
		h.BadRequestResponse(c, "invalid testee_id format", err)
		return 0, false
	}
	return testeeID, true
}

func (h *PersonalityAssessmentHandler) parseAssessmentID(c *gin.Context) (uint64, bool) {
	assessmentID, err := strconv.ParseUint(h.GetPathParam(c, "id"), 10, 64)
	if err != nil {
		h.BadRequestResponse(c, "invalid assessment id", err)
		return 0, false
	}
	return assessmentID, true
}
