package handler

import (
	"strconv"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/answersheet"
	"github.com/gin-gonic/gin"
)

// AnswerSheetHandler 答卷处理器
type AnswerSheetHandler struct {
	*BaseHandler
	submissionService *answersheet.SubmissionService
}

// NewAnswerSheetHandler 创建答卷处理器
func NewAnswerSheetHandler(submissionService *answersheet.SubmissionService) *AnswerSheetHandler {
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
// @Success 200 {object} core.Response{data=answersheet.SubmitAnswerSheetResponse}
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Security Bearer
// @Router /api/v1/answersheets [post]
func (h *AnswerSheetHandler) Submit(c *gin.Context) {
	var req answersheet.SubmitAnswerSheetRequest
	if err := h.BindJSON(c, &req); err != nil {
		return
	}

	// 从上下文获取当前用户ID（由 UserIdentityMiddleware 设置）
	writerID := h.GetUserID(c)
	if writerID == 0 {
		h.UnauthorizedResponse(c, "user not authenticated")
		return
	}

	result, err := h.submissionService.Submit(c.Request.Context(), writerID, &req)
	if err != nil {
		h.InternalErrorResponse(c, "save answer sheet failed", err)
		return
	}

	h.SuccessResponse(c, result)
}

// Get 获取答卷详情
// @Summary 获取答卷详情
// @Description 根据ID获取答卷详情
// @Tags 答卷
// @Produce json
// @Param id path int true "答卷ID"
// @Success 200 {object} core.Response{data=answersheet.AnswerSheetResponse}
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

	h.SuccessResponse(c, result)
}
