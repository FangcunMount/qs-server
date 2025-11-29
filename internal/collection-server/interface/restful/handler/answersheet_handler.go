package handler

import (
	"net/http"
	"strconv"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/answersheet"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/pkg/core"
	"github.com/gin-gonic/gin"
)

// UserIDKey 用户ID在context中的key
const UserIDKey = "user_id"

// GetUserID 从 gin.Context 获取用户ID
func GetUserID(c *gin.Context) uint64 {
	val, exists := c.Get(UserIDKey)
	if !exists {
		return 0
	}
	if id, ok := val.(uint64); ok {
		return id
	}
	return 0
}

// AnswerSheetHandler 答卷处理器
type AnswerSheetHandler struct {
	submissionService *answersheet.SubmissionService
}

// NewAnswerSheetHandler 创建答卷处理器
func NewAnswerSheetHandler(submissionService *answersheet.SubmissionService) *AnswerSheetHandler {
	return &AnswerSheetHandler{
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
// @Success 200 {object} answersheet.SubmitAnswerSheetResponse
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Security Bearer
// @Router /api/v1/answersheets [post]
func (h *AnswerSheetHandler) Submit(c *gin.Context) {
	var req answersheet.SubmitAnswerSheetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		core.WriteResponse(c, errors.WithCode(code.ErrBind, "bind request failed: %v", err), nil)
		return
	}

	// 从上下文获取当前用户ID
	writerID := GetUserID(c)
	if writerID == 0 {
		core.WriteResponse(c, errors.WithCode(code.ErrTokenInvalid, "user not authenticated"), nil)
		return
	}

	result, err := h.submissionService.Submit(c.Request.Context(), writerID, &req)
	if err != nil {
		core.WriteResponse(c, errors.WithCode(code.ErrDatabase, "save answer sheet failed: %v", err), nil)
		return
	}

	c.JSON(http.StatusOK, result)
}

// Get 获取答卷详情
// @Summary 获取答卷详情
// @Description 根据ID获取答卷详情
// @Tags 答卷
// @Produce json
// @Param id path int true "答卷ID"
// @Success 200 {object} answersheet.AnswerSheetResponse
// @Failure 400 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Security Bearer
// @Router /api/v1/answersheets/{id} [get]
func (h *AnswerSheetHandler) Get(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		core.WriteResponse(c, errors.WithCode(code.ErrBind, "invalid id format"), nil)
		return
	}

	result, err := h.submissionService.Get(c.Request.Context(), id)
	if err != nil {
		core.WriteResponse(c, errors.WithCode(code.ErrDatabase, "get answer sheet failed: %v", err), nil)
		return
	}

	if result == nil {
		core.WriteResponse(c, errors.WithCode(code.ErrPageNotFound, "answer sheet not found"), nil)
		return
	}

	c.JSON(http.StatusOK, result)
}
