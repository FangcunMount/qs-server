package questionnaire

import (
	"github.com/gin-gonic/gin"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/adapters/api/http/handlers"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/questionnaire"
)

// Handler 问卷HTTP处理器
type Handler struct {
	*handlers.BaseHandler
	questionnaireEditor *questionnaire.QuestionnaireEditor
	questionnaireQuery  *questionnaire.QuestionnaireQuery
}

// NewHandler 创建问卷处理器
func NewHandler(questionnaireEditor *questionnaire.QuestionnaireEditor, questionnaireQuery *questionnaire.QuestionnaireQuery) handlers.Handler {
	return &Handler{
		BaseHandler:         handlers.NewBaseHandler(),
		questionnaireEditor: questionnaireEditor,
		questionnaireQuery:  questionnaireQuery,
	}
}

// GetName 获取Handler名称
func (h *Handler) GetName() string {
	return "questionnaire"
}

// CreateQuestionnaireRequest 创建问卷请求
type CreateQuestionnaireRequest struct {
	Title       string `json:"title" binding:"required,min=2,max=200"`
	Description string `json:"description"`
}

// CreateQuestionnaire 创建问卷
// @Summary 创建问卷
// @Description 创建新的问卷
// @Tags questionnaires
// @Accept json
// @Produce json
// @Param questionnaire body CreateQuestionnaireRequest true "问卷信息"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/questionnaires [post]
func (h *Handler) CreateQuestionnaire(c *gin.Context) {
	var req CreateQuestionnaireRequest
	if err := h.BindJSON(c, &req); err != nil {
		return
	}

	// TODO: 从认证上下文获取用户ID
	creatorID := "user-123" // 暂时硬编码

	questionnaire, err := h.questionnaireEditor.CreateQuestionnaire(c.Request.Context(), req.Title, req.Description, creatorID)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponseWithMessage(c, "创建成功", h.questionnaireToResponse(questionnaire))
}

// GetQuestionnaire 获取问卷详情
// @Summary 获取问卷详情
// @Description 根据ID获取问卷详情
// @Tags questionnaires
// @Accept json
// @Produce json
// @Param id path string true "问卷ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/questionnaires/{id} [get]
func (h *Handler) GetQuestionnaire(c *gin.Context) {
	id := h.GetPathParam(c, "id")

	questionnaire, err := h.questionnaireQuery.GetQuestionnaireByID(c.Request.Context(), id)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponseWithMessage(c, "获取成功", h.questionnaireToResponse(questionnaire))
}

// GetQuestionnaireByCode 根据代码获取问卷
// @Summary 根据代码获取问卷
// @Description 根据问卷代码获取问卷详情
// @Tags questionnaires
// @Accept json
// @Produce json
// @Param code path string true "问卷代码"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/questionnaires/code/{code} [get]
func (h *Handler) GetQuestionnaireByCode(c *gin.Context) {
	code := h.GetPathParam(c, "code")

	questionnaire, err := h.questionnaireQuery.GetQuestionnaireByCode(c.Request.Context(), code)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponseWithMessage(c, "获取成功", h.questionnaireToResponse(questionnaire))
}

// ListQuestionnairesRequest 列表查询请求
type ListQuestionnairesRequest struct {
	Page      int    `form:"page,default=1"`
	PageSize  int    `form:"page_size,default=20"`
	Status    string `form:"status,default=all"`
	CreatorID string `form:"creator_id"`
	Keyword   string `form:"keyword"`
	SortBy    string `form:"sort_by,default=updated_at"`
	SortDir   string `form:"sort_dir,default=desc"`
}

// ListQuestionnaires 获取问卷列表
// @Summary 获取问卷列表
// @Description 分页获取问卷列表
// @Tags questionnaires
// @Accept json
// @Produce json
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Param status query string false "状态筛选"
// @Param creator_id query string false "创建者ID"
// @Param keyword query string false "搜索关键字"
// @Param sort_by query string false "排序字段"
// @Param sort_dir query string false "排序方向" Enums(asc, desc)
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/questionnaires [get]
func (h *Handler) ListQuestionnaires(c *gin.Context) {
	var req ListQuestionnairesRequest
	if err := h.BindQuery(c, &req); err != nil {
		return
	}

	query := questionnaire.QuestionnaireListQuery{
		Page:      req.Page,
		PageSize:  req.PageSize,
		Status:    req.Status,
		CreatorID: req.CreatorID,
		Keyword:   req.Keyword,
		SortBy:    req.SortBy,
		SortDir:   req.SortDir,
	}

	result, err := h.questionnaireQuery.GetQuestionnaireList(c.Request.Context(), query)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	// 转换为响应格式
	items := make([]map[string]interface{}, len(result.Questionnaires))
	for i, q := range result.Questionnaires {
		items[i] = h.questionnaireToResponse(q)
	}

	data := gin.H{
		"items": items,
		"pagination": gin.H{
			"total":       result.Total,
			"page":        result.Page,
			"page_size":   result.PageSize,
			"total_pages": result.TotalPages,
		},
	}

	h.SuccessResponseWithMessage(c, "获取成功", data)
}

// UpdateQuestionnaireRequest 更新问卷请求
type UpdateQuestionnaireRequest struct {
	Title       string `json:"title" binding:"required,min=2,max=200"`
	Description string `json:"description"`
}

// UpdateQuestionnaire 更新问卷
// @Summary 更新问卷
// @Description 更新问卷基础信息
// @Tags questionnaires
// @Accept json
// @Produce json
// @Param id path string true "问卷ID"
// @Param questionnaire body UpdateQuestionnaireRequest true "问卷更新信息"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/questionnaires/{id} [put]
func (h *Handler) UpdateQuestionnaire(c *gin.Context) {
	var req UpdateQuestionnaireRequest
	if err := h.BindJSON(c, &req); err != nil {
		return
	}

	id := h.GetPathParam(c, "id")

	questionnaire, err := h.questionnaireEditor.UpdateQuestionnaireInfo(c.Request.Context(), id, req.Title, req.Description)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponseWithMessage(c, "更新成功", h.questionnaireToResponse(questionnaire))
}

// PublishQuestionnaire 发布问卷
// @Summary 发布问卷
// @Description 发布问卷，使其可以被填写
// @Tags questionnaires
// @Accept json
// @Produce json
// @Param id path string true "问卷ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/questionnaires/{id}/publish [post]
func (h *Handler) PublishQuestionnaire(c *gin.Context) {
	id := h.GetPathParam(c, "id")

	questionnaire, err := h.questionnaireEditor.PublishQuestionnaire(c.Request.Context(), id)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponseWithMessage(c, "发布成功", h.questionnaireToResponse(questionnaire))
}

// UnpublishQuestionnaire 取消发布问卷
// @Summary 取消发布问卷
// @Description 将已发布的问卷撤回到草稿状态
// @Tags questionnaires
// @Accept json
// @Produce json
// @Param id path string true "问卷ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/questionnaires/{id}/unpublish [post]
func (h *Handler) UnpublishQuestionnaire(c *gin.Context) {
	id := h.GetPathParam(c, "id")

	questionnaire, err := h.questionnaireEditor.UnpublishQuestionnaire(c.Request.Context(), id)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponseWithMessage(c, "取消发布成功", h.questionnaireToResponse(questionnaire))
}

// ArchiveQuestionnaire 归档问卷
// @Summary 归档问卷
// @Description 将问卷归档，不再使用
// @Tags questionnaires
// @Accept json
// @Produce json
// @Param id path string true "问卷ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/questionnaires/{id}/archive [post]
func (h *Handler) ArchiveQuestionnaire(c *gin.Context) {
	id := h.GetPathParam(c, "id")

	err := h.questionnaireEditor.ArchiveQuestionnaire(c.Request.Context(), id)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponseWithMessage(c, "归档成功", nil)
}

// DeleteQuestionnaire 删除问卷
// @Summary 删除问卷
// @Description 删除指定的问卷
// @Tags questionnaires
// @Accept json
// @Produce json
// @Param id path string true "问卷ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/questionnaires/{id} [delete]
func (h *Handler) DeleteQuestionnaire(c *gin.Context) {
	id := h.GetPathParam(c, "id")

	err := h.questionnaireEditor.DeleteQuestionnaire(c.Request.Context(), id)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponseWithMessage(c, "删除成功", nil)
}

// AddQuestionRequest 添加问题请求
type AddQuestionRequest struct {
	QuestionText string   `json:"question_text" binding:"required"`
	QuestionType string   `json:"question_type" binding:"required"`
	Options      []string `json:"options"`
}

// AddQuestion 添加问题到问卷
// @Summary 添加问题到问卷
// @Description 向问卷添加新问题
// @Tags questionnaires
// @Accept json
// @Produce json
// @Param id path string true "问卷ID"
// @Param question body AddQuestionRequest true "问题信息"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/questionnaires/{id}/questions [post]
func (h *Handler) AddQuestion(c *gin.Context) {
	var req AddQuestionRequest
	if err := h.BindJSON(c, &req); err != nil {
		return
	}

	id := h.GetPathParam(c, "id")

	questionnaire, err := h.questionnaireEditor.AddQuestion(c.Request.Context(), id, req.QuestionText, req.QuestionType, req.Options)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponseWithMessage(c, "添加问题成功", h.questionnaireToResponse(questionnaire))
}

// RemoveQuestion 从问卷移除问题
// @Summary 从问卷移除问题
// @Description 从问卷中删除指定问题
// @Tags questionnaires
// @Accept json
// @Produce json
// @Param id path string true "问卷ID"
// @Param questionId path string true "问题ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/questionnaires/{id}/questions/{questionId} [delete]
func (h *Handler) RemoveQuestion(c *gin.Context) {
	id := h.GetPathParam(c, "id")
	questionID := h.GetPathParam(c, "questionId")

	questionnaire, err := h.questionnaireEditor.RemoveQuestion(c.Request.Context(), id, questionID)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponseWithMessage(c, "移除问题成功", h.questionnaireToResponse(questionnaire))
}

// GetQuestionnaireStats 获取问卷统计信息
// @Summary 获取问卷统计信息
// @Description 获取问卷的统计数据
// @Tags questionnaires
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/questionnaires/stats [get]
func (h *Handler) GetQuestionnaireStats(c *gin.Context) {
	stats, err := h.questionnaireQuery.GetQuestionnaireStats(c.Request.Context())
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponseWithMessage(c, "获取成功", stats)
}

// GetMyQuestionnaires 获取我的问卷列表
// @Summary 获取我的问卷列表
// @Description 获取当前用户创建的问卷列表
// @Tags questionnaires
// @Accept json
// @Produce json
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/questionnaires/my [get]
func (h *Handler) GetMyQuestionnaires(c *gin.Context) {
	page := 1
	pageSize := 20

	if p := c.Query("page"); p != "" {
		if parsed, err := c.GetQuery("page"); err {
			page = 1
		} else {
			page = int(parsed[0] - '0') // 简单转换
		}
	}

	if ps := c.Query("page_size"); ps != "" {
		if parsed, err := c.GetQuery("page_size"); err {
			pageSize = 20
		} else {
			pageSize = int(parsed[0] - '0') // 简单转换
		}
	}

	// TODO: 从认证上下文获取用户ID
	userID := "user-123" // 暂时硬编码

	result, err := h.questionnaireQuery.GetUserQuestionnaires(c.Request.Context(), userID, page, pageSize)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	// 转换为响应格式
	items := make([]map[string]interface{}, len(result.Questionnaires))
	for i, q := range result.Questionnaires {
		items[i] = h.questionnaireToResponse(q)
	}

	data := gin.H{
		"items": items,
		"pagination": gin.H{
			"total":       result.Total,
			"page":        result.Page,
			"page_size":   result.PageSize,
			"total_pages": result.TotalPages,
		},
	}

	h.SuccessResponseWithMessage(c, "获取成功", data)
}

// GetPublishedQuestionnaires 获取已发布的问卷列表
// @Summary 获取已发布的问卷列表
// @Description 获取公开发布的问卷列表
// @Tags questionnaires
// @Accept json
// @Produce json
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Param keyword query string false "搜索关键字"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/questionnaires/published [get]
func (h *Handler) GetPublishedQuestionnaires(c *gin.Context) {
	page := 1
	pageSize := 20
	keyword := c.Query("keyword")

	if p := c.Query("page"); p != "" {
		// 简单解析，实际应用中需要更严格的验证
		page = 1
	}

	if ps := c.Query("page_size"); ps != "" {
		pageSize = 20
	}

	result, err := h.questionnaireQuery.GetPublishedQuestionnaires(c.Request.Context(), page, pageSize, keyword)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	// 转换为响应格式
	items := make([]map[string]interface{}, len(result.Questionnaires))
	for i, q := range result.Questionnaires {
		items[i] = h.questionnaireToResponse(q)
	}

	data := gin.H{
		"items": items,
		"pagination": gin.H{
			"total":       result.Total,
			"page":        result.Page,
			"page_size":   result.PageSize,
			"total_pages": result.TotalPages,
		},
	}

	h.SuccessResponseWithMessage(c, "获取成功", data)
}

// questionnaireToResponse 将DTO转换为响应格式
func (h *Handler) questionnaireToResponse(q *questionnaire.QuestionnaireDTO) map[string]interface{} {
	return map[string]interface{}{
		"id":          q.ID,
		"code":        q.Code,
		"title":       q.Title,
		"description": q.Description,
		"status":      q.Status,
		"creator_id":  q.CreatedBy,
		"created_at":  q.CreatedAt,
		"updated_at":  q.UpdatedAt,
		"questions":   q.Questions,
	}
}
