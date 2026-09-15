package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	app "github.com/FangcunMount/qs-server/internal/collection-server/application/aiexplanation"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/ratelimit"
	"github.com/FangcunMount/qs-server/pkg/core"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

type aiExplanationService interface {
	RequestWorkflow(context.Context, uint64, uint64, app.WorkflowRequest) (*app.WorkflowAccepted, error)
	GetWorkflow(context.Context, uint64, uint64, string) (*app.WorkflowResult, error)
	GetWorkflowSource(context.Context, uint64, uint64) (*app.WorkflowSource, error)
}

type AIExplanationHandler struct {
	*BaseHandler
	service aiExplanationService
}

func NewAIExplanationHandler(service aiExplanationService) *AIExplanationHandler {
	return &AIExplanationHandler{BaseHandler: NewBaseHandler(), service: service}
}

func (h *AIExplanationHandler) parseIdentity(c *gin.Context) (uint64, uint64, bool) {
	testeeID, err := strconv.ParseUint(strings.TrimSpace(c.Query("testee_id")), 10, 64)
	if err != nil || testeeID == 0 {
		h.BadRequestResponse(c, "valid testee_id is required", err)
		return 0, 0, false
	}
	assessmentID, err := strconv.ParseUint(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || assessmentID == 0 {
		h.BadRequestResponse(c, "valid assessment id is required", err)
		return 0, 0, false
	}
	return testeeID, assessmentID, true
}

func (h *AIExplanationHandler) respondError(c *gin.Context, err error) {
	if errors.Is(err, app.ErrInvalidRequest) {
		h.BadRequestResponse(c, "invalid AI explanation request", nil)
		return
	}
	if errors.Is(err, app.ErrUnavailable) {
		c.JSON(http.StatusServiceUnavailable, core.ErrResponse{Code: http.StatusServiceUnavailable, Message: "AI explanation temporarily unavailable"})
		return
	}
	switch grpcstatus.Code(err) {
	case codes.InvalidArgument:
		h.BadRequestResponse(c, "invalid AI explanation request", nil)
	case codes.Unauthenticated:
		h.UnauthorizedResponse(c, "user not authenticated")
	case codes.PermissionDenied:
		h.ForbiddenResponse(c, "AI explanation access denied")
	case codes.NotFound:
		h.NotFoundResponse(c, "AI explanation not found", nil)
	case codes.FailedPrecondition, codes.Aborted, codes.AlreadyExists:
		h.ConflictResponse(c, "AI explanation request cannot be completed", nil)
	case codes.ResourceExhausted:
		ratelimit.ApplyRetryAfterSeconds(c.Writer.Header(), secondsUntilNextUTCDate(time.Now()))
		c.JSON(http.StatusTooManyRequests, core.ErrResponse{Code: http.StatusTooManyRequests, Message: "AI explanation daily capacity exceeded"})
	case codes.Unavailable, codes.DeadlineExceeded:
		c.JSON(http.StatusServiceUnavailable, core.ErrResponse{Code: http.StatusServiceUnavailable, Message: "AI explanation temporarily unavailable"})
	default:
		h.InternalErrorResponse(c, "AI explanation request failed", err)
	}
}

func secondsUntilNextUTCDate(now time.Time) int {
	now = now.UTC()
	next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
	remaining := next.Sub(now)
	seconds := int((remaining + time.Second - 1) / time.Second)
	if seconds < 1 {
		return 1
	}
	return seconds
}

// RequestWorkflow queues a snapshot task through the existing participant identity boundary.
// @Summary 请求新版 AI 报告快照任务
// @Tags AI解读
// @Accept json
// @Produce json
// @Param id path int true "测评ID"
// @Param testee_id query int true "受试者ID"
// @Param request body app.WorkflowRequest true "稳定请求ID及标准报告ID"
// @Success 202 {object} core.Response{data=app.WorkflowAccepted}
// @Failure 400 {object} core.ErrResponse
// @Failure 403 {object} core.ErrResponse
// @Failure 503 {object} core.ErrResponse
// @Security BearerAuth
// @Router /api/v1/assessments/{id}/ai-workflows [post]
func (h *AIExplanationHandler) RequestWorkflow(c *gin.Context) {
	testeeID, assessmentID, ok := h.parseIdentity(c)
	if !ok {
		return
	}
	var request app.WorkflowRequest
	if err := h.BindJSON(c, &request); err != nil {
		return
	}
	service := h.service
	if service == nil {
		h.respondError(c, app.ErrUnavailable)
		return
	}
	result, err := service.RequestWorkflow(c.Request.Context(), testeeID, assessmentID, request)
	if err != nil {
		h.respondError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, core.Response{Code: 0, Message: "accepted", Data: result})
}

// GetWorkflow reads a durable AI result with current participant authorization.
// @Summary 读取 AI 工作流结果
// @Tags AI解读
// @Produce json
// @Param id path string true "测评ID"
// @Param request_id path string true "工作流请求UUID"
// @Param testee_id query string true "受试者ID"
// @Success 200 {object} core.Response{data=app.WorkflowResult}
// @Failure 400 {object} core.ErrResponse
// @Failure 403 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 503 {object} core.ErrResponse
// @Security BearerAuth
// @Router /api/v1/assessments/{id}/ai-workflows/{request_id} [get]
func (h *AIExplanationHandler) GetWorkflow(c *gin.Context) {
	testeeID, assessmentID, ok := h.parseIdentity(c)
	if !ok {
		return
	}
	service := h.service
	if service == nil {
		h.respondError(c, app.ErrUnavailable)
		return
	}
	result, err := service.GetWorkflow(c.Request.Context(), testeeID, assessmentID, c.Param("request_id"))
	if err != nil {
		h.respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, core.Response{Code: 0, Message: "success", Data: result})
}

// GetWorkflowSource returns the current report identity after participant authorization.
// @Summary 查询新版 AI 工作流的当前报告来源
// @Tags AI解读
// @Produce json
// @Param id path int true "测评ID"
// @Param testee_id query int true "受试者ID"
// @Success 200 {object} core.Response{data=app.WorkflowSource}
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 403 {object} core.ErrResponse
// @Failure 503 {object} core.ErrResponse
// @Security BearerAuth
// @Router /api/v1/assessments/{id}/ai-workflows/source [get]
func (h *AIExplanationHandler) GetWorkflowSource(c *gin.Context) {
	testeeID, assessmentID, ok := h.parseIdentity(c)
	if !ok {
		return
	}
	service := h.service
	if service == nil {
		h.respondError(c, app.ErrUnavailable)
		return
	}
	result, err := service.GetWorkflowSource(c.Request.Context(), testeeID, assessmentID)
	if err != nil {
		h.respondError(c, err)
		return
	}
	h.Success(c, result)
}
