package handler

import (
	"github.com/FangcunMount/qs-server/internal/collection-server/application/questionnaire"
	"github.com/gin-gonic/gin"
)

// QuestionnaireHandler 问卷处理器
type QuestionnaireHandler struct {
	*BaseHandler
	queryService *questionnaire.QueryService
}

// NewQuestionnaireHandler 创建问卷处理器
func NewQuestionnaireHandler(queryService *questionnaire.QueryService) *QuestionnaireHandler {
	return &QuestionnaireHandler{
		BaseHandler:  NewBaseHandler(),
		queryService: queryService,
	}
}

// Get 获取问卷详情
// @Summary 获取问卷详情
// @Description 根据问卷编码获取问卷详情
// @Tags 问卷
// @Produce json
// @Param code path string true "问卷编码"
// @Success 200 {object} core.Response{data=questionnaire.QuestionnaireResponse}
// @Failure 400 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /api/v1/questionnaires/{code} [get]
func (h *QuestionnaireHandler) Get(c *gin.Context) {
	qcode := c.Param("code")
	if qcode == "" {
		h.BadRequestResponse(c, "code is required", nil)
		return
	}

	result, err := h.queryService.Get(c.Request.Context(), qcode)
	if err != nil {
		h.InternalErrorResponse(c, "get questionnaire failed", err)
		return
	}

	if result == nil {
		h.NotFoundResponse(c, "questionnaire not found", nil)
		return
	}

	h.Success(c, result)
}

// List 获取问卷列表
// @Summary 获取问卷列表
// @Description 分页获取问卷列表
// @Tags 问卷
// @Produce json
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Param status query int false "状态过滤（0=草稿, 1=已发布, 2=已归档）"
// @Param title query string false "标题过滤"
// @Success 200 {object} core.Response{data=questionnaire.ListQuestionnairesResponse}
// @Failure 400 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /api/v1/questionnaires [get]
func (h *QuestionnaireHandler) List(c *gin.Context) {
	var req questionnaire.ListQuestionnairesRequest
	if err := h.BindQuery(c, &req); err != nil {
		return
	}

	result, err := h.queryService.List(c.Request.Context(), &req)
	if err != nil {
		h.InternalErrorResponse(c, "list questionnaires failed", err)
		return
	}

	h.Success(c, result)
}
