package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/answersheet"
	pkgmiddleware "github.com/FangcunMount/qs-server/internal/pkg/middleware"
	"github.com/FangcunMount/qs-server/pkg/core"
	"github.com/gin-gonic/gin"
	uuid "github.com/satori/go.uuid"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

type answerSheetSubmissionService interface {
	SubmitQueued(ctx context.Context, requestID string, writerID uint64, req *answersheet.SubmitAnswerSheetRequest) error
	GetSubmitStatus(requestID string) (*answersheet.SubmitStatusResponse, bool)
	Get(ctx context.Context, id uint64) (*answersheet.AnswerSheetResponse, error)
}

// AnswerSheetHandler 答卷处理器
type AnswerSheetHandler struct {
	*BaseHandler
	submissionService answerSheetSubmissionService
}

// NewAnswerSheetHandler 创建答卷处理器
func NewAnswerSheetHandler(submissionService answerSheetSubmissionService) *AnswerSheetHandler {
	return &AnswerSheetHandler{
		BaseHandler:       NewBaseHandler(),
		submissionService: submissionService,
	}
}

// Submit 提交答卷
// @Summary 提交答卷
// @Description 用户提交问卷答卷
// @Tags 答卷
// @Accept json
// @Produce json
// @Param request body answersheet.SubmitAnswerSheetRequest true "答卷数据"
// @Success 202 {object} core.Response{data=answersheet.SubmitAcceptedResponse}
// @Failure 429 {object} core.ErrResponse
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Security Bearer
// @Router /api/v1/answersheets [post]
func (h *AnswerSheetHandler) Submit(c *gin.Context) {
	var req answersheet.SubmitAnswerSheetRequest
	if err := h.BindJSON(c, &req); err != nil {
		return // BindJSON 已包含 binding 标签校验
	}
	if req.TaskID == "" {
		req.TaskID = c.Query("task_id")
	}

	// 从上下文获取当前用户ID（由 UserIdentityMiddleware 设置）
	writerID := h.GetUserID(c)
	if writerID == 0 {
		h.UnauthorizedResponse(c, "user not authenticated")
		return
	}

	requestID := pkgmiddleware.GetRequestIDFromContext(c)
	if requestID == "" {
		requestID = pkgmiddleware.GetRequestIDFromHeaders(c)
	}
	if requestID == "" {
		requestID = uuid.Must(uuid.NewV4(), nil).String()
	}

	if err := h.submissionService.SubmitQueued(c.Request.Context(), requestID, writerID, &req); err != nil {
		if errors.Is(err, answersheet.ErrQueueFull) {
			c.JSON(http.StatusTooManyRequests, core.ErrResponse{
				Code:    http.StatusTooManyRequests,
				Message: "submit queue full",
			})
			return
		}
		h.respondSubmitError(c, err)
		return
	}

	c.JSON(http.StatusAccepted, core.Response{
		Code:    0,
		Message: "accepted",
		Data: answersheet.SubmitAcceptedResponse{
			Status:    answersheet.SubmitStatusQueued,
			RequestID: requestID,
		},
	})
}

func (h *AnswerSheetHandler) respondSubmitError(c *gin.Context, err error) {
	st, ok := grpcstatus.FromError(err)
	if !ok {
		h.InternalErrorResponse(c, "save answer sheet failed", err)
		return
	}

	switch st.Code() {
	case codes.InvalidArgument:
		c.JSON(http.StatusBadRequest, core.ErrResponse{
			Code:    http.StatusBadRequest,
			Message: st.Message(),
		})
	case codes.PermissionDenied:
		c.JSON(http.StatusForbidden, core.ErrResponse{
			Code:    http.StatusForbidden,
			Message: st.Message(),
		})
	case codes.NotFound:
		c.JSON(http.StatusNotFound, core.ErrResponse{
			Code:    http.StatusNotFound,
			Message: st.Message(),
		})
	case codes.Unauthenticated:
		c.JSON(http.StatusUnauthorized, core.ErrResponse{
			Code:    http.StatusUnauthorized,
			Message: st.Message(),
		})
	case codes.ResourceExhausted:
		c.JSON(http.StatusTooManyRequests, core.ErrResponse{
			Code:    http.StatusTooManyRequests,
			Message: st.Message(),
		})
	default:
		h.InternalErrorResponse(c, "save answer sheet failed", err)
	}
}

// SubmitStatus 查询提交状态
// @Summary 查询提交状态
// @Description 根据 request_id 查询提交处理状态
// @Tags 答卷
// @Produce json
// @Param request_id query string true "请求ID"
// @Success 200 {object} core.Response{data=answersheet.SubmitStatusResponse}
// @Failure 429 {object} core.ErrResponse
// @Failure 400 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Security Bearer
// @Router /api/v1/answersheets/submit-status [get]
func (h *AnswerSheetHandler) SubmitStatus(c *gin.Context) {
	requestID := h.GetQueryParam(c, "request_id")
	if requestID == "" {
		h.BadRequestResponse(c, "request_id is required", nil)
		return
	}

	status, ok := h.submissionService.GetSubmitStatus(requestID)
	if !ok {
		h.NotFoundResponse(c, "submit status not found", nil)
		return
	}

	h.Success(c, status)
}

// Get 获取答卷详情
// @Summary 获取答卷详情
// @Description 根据ID获取答卷详情
// @Tags 答卷
// @Produce json
// @Param id path int true "答卷ID"
// @Success 200 {object} core.Response{data=answersheet.AnswerSheetResponse}
// @Failure 429 {object} core.ErrResponse
// @Failure 400 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Security Bearer
// @Router /api/v1/answersheets/{id} [get]
func (h *AnswerSheetHandler) Get(c *gin.Context) {
	idStr := h.GetPathParam(c, "id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		h.BadRequestResponse(c, "invalid id format", err)
		return
	}

	result, err := h.submissionService.Get(c.Request.Context(), id)
	if err != nil {
		h.InternalErrorResponse(c, "get answer sheet failed", err)
		return
	}

	if result == nil {
		h.NotFoundResponse(c, "answer sheet not found", nil)
		return
	}

	h.Success(c, result)
}
