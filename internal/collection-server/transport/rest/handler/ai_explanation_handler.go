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
	Capability(context.Context, uint64, uint64, app.Request) (*app.Response, error)
	Request(context.Context, uint64, uint64, app.Request) (*app.Response, error)
	Get(context.Context, uint64, uint64, string) (*app.Response, error)
	Export(context.Context, uint64, int, string) (*app.ExportPage, error)
}

type AIExplanationHandler struct {
	*BaseHandler
	service aiExplanationService
}

func NewAIExplanationHandler(service aiExplanationService) *AIExplanationHandler {
	return &AIExplanationHandler{BaseHandler: NewBaseHandler(), service: service}
}

// Capability reports whether the current immutable standard report can be
// supplemented by the published AI explanation profile.
// @Summary 查询 AI 解读能力
// @Tags AI解读
// @Produce json
// @Param id path int true "测评ID"
// @Param testee_id query int true "受试者ID"
// @Param locale query string false "输出语言" default(zh-CN)
// @Param focus_area query []string false "关注领域，可重复"
// @Success 200 {object} core.Response{data=app.Response}
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 403 {object} core.ErrResponse
// @Failure 503 {object} core.ErrResponse
// @Security BearerAuth
// @Router /api/v1/assessments/{id}/ai-explanation/capability [get]
func (h *AIExplanationHandler) Capability(c *gin.Context) {
	testeeID, assessmentID, ok := h.parseIdentity(c)
	if !ok {
		return
	}
	result, err := h.service.Capability(c.Request.Context(), testeeID, assessmentID, app.Request{
		Locale: c.DefaultQuery("locale", "zh-CN"), FocusAreas: c.QueryArray("focus_area"),
	})
	if err != nil {
		h.respondError(c, err)
		return
	}
	h.Success(c, result)
}

// Request manually starts an AI explanation generation. It never regenerates
// or changes the standard report.
// @Summary 手动请求 AI 解读
// @Tags AI解读
// @Accept json
// @Produce json
// @Param id path int true "测评ID"
// @Param testee_id query int true "受试者ID"
// @Param request body app.Request true "AI 解读范围"
// @Success 200 {object} core.Response{data=app.Response}
// @Success 202 {object} core.Response{data=app.Response}
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 403 {object} core.ErrResponse
// @Failure 429 {object} core.ErrResponse
// @Failure 503 {object} core.ErrResponse
// @Security BearerAuth
// @Router /api/v1/assessments/{id}/ai-explanations [post]
func (h *AIExplanationHandler) Request(c *gin.Context) {
	testeeID, assessmentID, ok := h.parseIdentity(c)
	if !ok {
		return
	}
	var request app.Request
	if err := h.BindJSON(c, &request); err != nil {
		return
	}
	result, err := h.service.Request(c.Request.Context(), testeeID, assessmentID, request)
	if err != nil {
		h.respondError(c, err)
		return
	}
	if result != nil && (result.Status == "pending" || result.Status == "generating") {
		c.JSON(http.StatusAccepted, core.Response{Code: 0, Message: "accepted", Data: result})
		return
	}
	h.Success(c, result)
}

// Get returns lifecycle state or a validated immutable AI explanation artifact.
// @Summary 查询 AI 解读
// @Tags AI解读
// @Produce json
// @Param id path int true "测评ID"
// @Param generation_id path string true "AI 解读 Generation ID"
// @Param testee_id query int true "受试者ID"
// @Success 200 {object} core.Response{data=app.Response}
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 403 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 503 {object} core.ErrResponse
// @Security BearerAuth
// @Router /api/v1/assessments/{id}/ai-explanations/{generation_id} [get]
func (h *AIExplanationHandler) Get(c *gin.Context) {
	testeeID, assessmentID, ok := h.parseIdentity(c)
	if !ok {
		return
	}
	generationID := strings.TrimSpace(c.Param("generation_id"))
	if generationID == "" {
		h.BadRequestResponse(c, "AI explanation generation id is required", nil)
		return
	}
	result, err := h.service.Get(c.Request.Context(), testeeID, assessmentID, generationID)
	if err != nil {
		h.respondError(c, err)
		return
	}
	h.Success(c, result)
}

// Export returns a stable, participant-authorized page of final AI
// explanation artifacts and release provenance. Internal prompts, runs,
// Provider invocation receipts and capacity ledgers are never included.
// @Summary 导出本人 AI 解读数据
// @Tags AI解读
// @Produce json
// @Param testee_id query int true "受试者ID"
// @Param page_size query int false "每页条数，1-100" default(50)
// @Param cursor query string false "上一页返回的游标"
// @Success 200 {object} core.Response{data=app.ExportPage}
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 403 {object} core.ErrResponse
// @Failure 503 {object} core.ErrResponse
// @Security BearerAuth
// @Router /api/v1/ai-explanations/export [get]
func (h *AIExplanationHandler) Export(c *gin.Context) {
	testeeID, err := strconv.ParseUint(strings.TrimSpace(c.Query("testee_id")), 10, 64)
	if err != nil || testeeID == 0 {
		h.BadRequestResponse(c, "valid testee_id is required", err)
		return
	}
	pageSize := 0
	if raw := strings.TrimSpace(c.Query("page_size")); raw != "" {
		pageSize, err = strconv.Atoi(raw)
		if err != nil || pageSize < 1 || pageSize > 100 {
			h.BadRequestResponse(c, "page_size must be between 1 and 100", err)
			return
		}
	}
	result, err := h.service.Export(c.Request.Context(), testeeID, pageSize, strings.TrimSpace(c.Query("cursor")))
	if err != nil {
		h.respondError(c, err)
		return
	}
	h.Success(c, result)
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
