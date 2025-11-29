package handler

import (
	"net/http"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/questionnaire"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/pkg/core"
	"github.com/gin-gonic/gin"
)

// QuestionnaireHandler 问卷处理器
type QuestionnaireHandler struct {
	queryService *questionnaire.QueryService
}

// NewQuestionnaireHandler 创建问卷处理器
func NewQuestionnaireHandler(queryService *questionnaire.QueryService) *QuestionnaireHandler {
	return &QuestionnaireHandler{
		queryService: queryService,
	}
}

// Get 获取问卷详情
// @Summary 获取问卷详情
// @Description 根据问卷编码获取问卷详情
// @Tags 问卷
// @Produce json
// @Param code path string true "问卷编码"
// @Success 200 {object} questionnaire.QuestionnaireResponse
// @Failure 400 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /api/v1/questionnaires/{code} [get]
func (h *QuestionnaireHandler) Get(c *gin.Context) {
	qcode := c.Param("code")
	if qcode == "" {
		core.WriteResponse(c, errors.WithCode(code.ErrBind, "code is required"), nil)
		return
	}

	result, err := h.queryService.Get(c.Request.Context(), qcode)
	if err != nil {
		core.WriteResponse(c, errors.WithCode(code.ErrDatabase, "get questionnaire failed: %v", err), nil)
		return
	}

	if result == nil {
		core.WriteResponse(c, errors.WithCode(code.ErrPageNotFound, "questionnaire not found"), nil)
		return
	}

	c.JSON(http.StatusOK, result)
}

// List 获取问卷列表
// @Summary 获取问卷列表
// @Description 分页获取问卷列表
// @Tags 问卷
// @Produce json
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Param status query string false "状态过滤"
// @Param title query string false "标题过滤"
// @Success 200 {object} questionnaire.ListQuestionnairesResponse
// @Failure 400 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /api/v1/questionnaires [get]
func (h *QuestionnaireHandler) List(c *gin.Context) {
	var req questionnaire.ListQuestionnairesRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		core.WriteResponse(c, errors.WithCode(code.ErrBind, "bind query failed: %v", err), nil)
		return
	}

	result, err := h.queryService.List(c.Request.Context(), &req)
	if err != nil {
		core.WriteResponse(c, errors.WithCode(code.ErrDatabase, "list questionnaires failed: %v", err), nil)
		return
	}

	c.JSON(http.StatusOK, result)
}
