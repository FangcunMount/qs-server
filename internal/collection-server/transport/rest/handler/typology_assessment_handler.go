package handler

import (
	"context"
	"strconv"
	"time"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/reportwait"
	typologyassessment "github.com/FangcunMount/qs-server/internal/collection-server/application/typologyassessment"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type typologyAssessmentQueryService interface {
	List(ctx context.Context, testeeID uint64, req *typologyassessment.ListAssessmentsRequest) (*typologyassessment.ListAssessmentsResponse, error)
	Get(ctx context.Context, testeeID, assessmentID uint64) (*typologyassessment.AssessmentDetailResponse, error)
	GetReport(ctx context.Context, testeeID, assessmentID uint64) (*typologyassessment.AssessmentReportResponse, error)
	GetReportStatus(ctx context.Context, testeeID, assessmentID uint64) (*typologyassessment.AssessmentStatusResponse, error)
	WaitReport(ctx context.Context, testeeID, assessmentID uint64, timeout time.Duration) (*typologyassessment.AssessmentStatusResponse, error)
}

type TypologyAssessmentHandler struct {
	*BaseHandler
	queryService      typologyAssessmentQueryService
	waitReportService *reportwait.Service
}

func NewTypologyAssessmentHandler(
	queryService typologyAssessmentQueryService,
	waitReportService *reportwait.Service,
) *TypologyAssessmentHandler {
	return &TypologyAssessmentHandler{
		BaseHandler:       NewBaseHandler(),
		queryService:      queryService,
		waitReportService: waitReportService,
	}
}

// List lists typology assessments for a testee.
// @Summary 查询类型学测评列表
// @Description 返回受试者人格测评列表。提交答卷后可用 items[].answer_sheet_id 与 submit-status 返回的 answersheet_id 匹配，取得 assessment_id（id 字段）。R121 后不再支持按答卷反查测评。
// @Tags 类型学测评
// @Produce json
// @Param testee_id query string true "受试者ID（建议字符串）"
// @Success 200 {object} core.Response{data=typologyassessment.ListAssessmentsResponse}
// @Router /api/v1/typology-assessments [get]
func (h *TypologyAssessmentHandler) List(c *gin.Context) {
	testeeID, ok := h.parseTesteeID(c)
	if !ok {
		return
	}
	var req typologyassessment.ListAssessmentsRequest
	if err := h.BindQuery(c, &req); err != nil {
		return
	}
	result, err := h.queryService.List(c.Request.Context(), testeeID, &req)
	if err != nil {
		h.InternalErrorResponse(c, "list typology assessments failed", err)
		return
	}
	h.Success(c, result)
}

// Get returns a typology assessment detail.
// @Summary 获取类型学测评详情
// @Tags 类型学测评
// @Produce json
// @Param id path int true "测评ID"
// @Param testee_id query string true "受试者ID（建议字符串）"
// @Success 200 {object} core.Response{data=typologyassessment.AssessmentDetailResponse}
// @Router /api/v1/typology-assessments/{id} [get]
func (h *TypologyAssessmentHandler) Get(c *gin.Context) {
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
		if typologyassessment.IsNotTypologyAssessment(err) {
			h.NotFoundResponse(c, "typology assessment not found", err)
			return
		}
		h.InternalErrorResponse(c, "get typology assessment failed", err)
		return
	}
	if result == nil {
		h.NotFoundResponse(c, "typology assessment not found", nil)
		return
	}
	h.Success(c, result)
}

// GetReport returns a typology assessment report.
// @Summary 获取类型学测评报告
// @Description 仅在 report-status 终态 interpreted 后调用。model.kind 规范值为 typology，读兼容历史 personality。
// @Tags 类型学测评
// @Produce json
// @Param id path int true "测评ID"
// @Param testee_id query string true "受试者ID（建议字符串）"
// @Success 200 {object} core.Response{data=typologyassessment.AssessmentReportResponse}
// @Failure 400 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /api/v1/typology-assessments/{id}/report [get]
func (h *TypologyAssessmentHandler) GetReport(c *gin.Context) {
	testeeID, ok := h.parseTesteeID(c)
	if !ok {
		return
	}
	assessmentID, ok := h.parseAssessmentID(c)
	if !ok {
		return
	}
	result, err := h.queryService.GetReport(c.Request.Context(), testeeID, assessmentID)
	if err != nil {
		if typologyassessment.IsNotTypologyAssessment(err) {
			h.NotFoundResponse(c, "typology assessment report not found", err)
			return
		}
		if status.Code(err) == codes.PermissionDenied {
			h.NotFoundResponse(c, "typology assessment report not found", nil)
			return
		}
		h.InternalErrorResponse(c, "get typology assessment report failed", err)
		return
	}
	if result == nil {
		h.NotFoundResponse(c, "typology assessment report not found", nil)
		return
	}
	h.Success(c, result)
}

// GetReportStatus 短轮询查询类型学测评报告状态（非阻塞）。
// @Summary 查询类型学测评报告状态
// @Description 推荐报告等待方式（优于 wait-report 长轮询）。非终态按 next_poll_after_ms 退避；亦可选用 WSS /api/v1/report-events（subscribe.kind=personality，需 report_events.enabled）。
// @Tags 类型学测评
// @Produce json
// @Param id path int true "测评ID"
// @Param testee_id query string true "受试者ID（建议字符串）"
// @Success 200 {object} core.Response{data=typologyassessment.AssessmentStatusResponse}
// @Router /api/v1/typology-assessments/{id}/report-status [get]
func (h *TypologyAssessmentHandler) GetReportStatus(c *gin.Context) {
	testeeID, ok := h.parseTesteeID(c)
	if !ok {
		return
	}
	assessmentID, ok := h.parseAssessmentID(c)
	if !ok {
		return
	}
	result, err := h.queryService.GetReportStatus(c.Request.Context(), testeeID, assessmentID)
	if err != nil {
		h.InternalErrorResponse(c, "get typology assessment report status failed", err)
		return
	}
	h.Success(c, result)
}

// WaitReport waits for a typology assessment report.
// @Summary 长轮询等待类型学测评报告
// @Description legacy 兼容；高并发下占连接。新接入请优先 report-status 短轮询或 WSS /report-events。
// @Tags 类型学测评
// @Produce json
// @Param id path int true "测评ID"
// @Param testee_id query string true "受试者ID（建议字符串）"
// @Param timeout query int false "超时时间（秒）" default(20)
// @Success 200 {object} core.Response{data=typologyassessment.AssessmentStatusResponse}
// @Router /api/v1/typology-assessments/{id}/wait-report [get]
func (h *TypologyAssessmentHandler) WaitReport(c *gin.Context) {
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
		h.InternalErrorResponse(c, "wait typology assessment report failed", err)
		return
	}
	h.Success(c, result)
}

func (h *TypologyAssessmentHandler) parseTesteeID(c *gin.Context) (uint64, bool) {
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

func (h *TypologyAssessmentHandler) parseAssessmentID(c *gin.Context) (uint64, bool) {
	assessmentID, err := strconv.ParseUint(h.GetPathParam(c, "id"), 10, 64)
	if err != nil {
		h.BadRequestResponse(c, "invalid assessment id", err)
		return 0, false
	}
	return assessmentID, true
}
