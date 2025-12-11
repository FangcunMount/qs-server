package handler

import (
	"strconv"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/request"
	"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/response"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/asaskevich/govalidator"
	"github.com/gin-gonic/gin"
)

// QuestionnaireHandler 问卷处理器
// 对接新的按行为者组织的应用服务层
type QuestionnaireHandler struct {
	BaseHandler
	lifecycleService questionnaire.QuestionnaireLifecycleService
	contentService   questionnaire.QuestionnaireContentService
	queryService     questionnaire.QuestionnaireQueryService
}

// NewQuestionnaireHandler 创建问卷处理器
func NewQuestionnaireHandler(
	lifecycleService questionnaire.QuestionnaireLifecycleService,
	contentService questionnaire.QuestionnaireContentService,
	queryService questionnaire.QuestionnaireQueryService,
) *QuestionnaireHandler {
	return &QuestionnaireHandler{
		lifecycleService: lifecycleService,
		contentService:   contentService,
		queryService:     queryService,
	}
}

// ============= Lifecycle API (生命周期管理) =============

// Create 创建问卷
// @Summary 创建问卷
// @Description 创建新问卷，初始状态为草稿
// @Tags Questionnaire-Lifecycle
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param request body request.CreateQuestionnaireRequest true "创建问卷请求"
// @Success 200 {object} handler.Response{data=response.QuestionnaireResponse}
// @Router /api/v1/questionnaires [post]
func (h *QuestionnaireHandler) Create(c *gin.Context) {
	var req request.CreateQuestionnaireRequest
	if err := h.BindJSON(c, &req); err != nil {
		h.ErrorResponse(c, err)
		return
	}
	if ok, err := govalidator.ValidateStruct(req); !ok {
		h.ErrorResponse(c, err)
		return
	}

	dto := questionnaire.CreateQuestionnaireDTO{
		Title:       req.Title,
		Description: req.Description,
		ImgUrl:      req.ImgUrl,
	}

	result, err := h.lifecycleService.Create(c.Request.Context(), dto)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, response.NewQuestionnaireResponseFromResult(result))
}

// UpdateBasicInfo 更新问卷基本信息
// @Summary 更新问卷基本信息
// @Description 更新问卷的标题、描述、封面图
// @Tags Questionnaire-Lifecycle
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "问卷编码"
// @Param request body request.UpdateQuestionnaireBasicInfoRequest true "更新请求"
// @Success 200 {object} handler.Response{data=response.QuestionnaireResponse}
// @Router /api/v1/questionnaires/{code}/basic-info [put]
func (h *QuestionnaireHandler) UpdateBasicInfo(c *gin.Context) {
	qCode := c.Param("code")
	if qCode == "" {
		h.ErrorResponse(c, errors.WithCode(code.ErrQuestionnaireInvalidInput, "问卷编码不能为空"))
		return
	}

	var req request.UpdateQuestionnaireBasicInfoRequest
	if err := h.BindJSON(c, &req); err != nil {
		h.ErrorResponse(c, err)
		return
	}
	if ok, err := govalidator.ValidateStruct(req); !ok {
		h.ErrorResponse(c, err)
		return
	}

	dto := questionnaire.UpdateQuestionnaireBasicInfoDTO{
		Code:        qCode,
		Title:       req.Title,
		Description: req.Description,
		ImgUrl:      req.ImgUrl,
	}

	result, err := h.lifecycleService.UpdateBasicInfo(c.Request.Context(), dto)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, response.NewQuestionnaireResponseFromResult(result))
}

// SaveDraft 保存草稿
// @Summary 保存草稿
// @Description 保存问卷为草稿状态，小版本号递增
// @Tags Questionnaire-Lifecycle
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "问卷编码"
// @Success 200 {object} handler.Response{data=response.QuestionnaireResponse}
// @Router /api/v1/questionnaires/{code}/draft [post]
func (h *QuestionnaireHandler) SaveDraft(c *gin.Context) {
	qCode := c.Param("code")
	if qCode == "" {
		h.ErrorResponse(c, errors.WithCode(code.ErrQuestionnaireInvalidInput, "问卷编码不能为空"))
		return
	}

	result, err := h.lifecycleService.SaveDraft(c.Request.Context(), qCode)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, response.NewQuestionnaireResponseFromResult(result))
}

// Publish 发布问卷
// @Summary 发布问卷
// @Description 发布问卷使其可用，大版本号递增
// @Tags Questionnaire-Lifecycle
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "问卷编码"
// @Success 200 {object} handler.Response{data=response.QuestionnaireResponse}
// @Router /api/v1/questionnaires/{code}/publish [post]
func (h *QuestionnaireHandler) Publish(c *gin.Context) {
	qCode := c.Param("code")
	if qCode == "" {
		h.ErrorResponse(c, errors.WithCode(code.ErrQuestionnaireInvalidInput, "问卷编码不能为空"))
		return
	}

	result, err := h.lifecycleService.Publish(c.Request.Context(), qCode)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, response.NewQuestionnaireResponseFromResult(result))
}

// Unpublish 下架问卷
// @Summary 下架问卷
// @Description 将已发布的问卷下架
// @Tags Questionnaire-Lifecycle
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "问卷编码"
// @Success 200 {object} handler.Response{data=response.QuestionnaireResponse}
// @Router /api/v1/questionnaires/{code}/unpublish [post]
func (h *QuestionnaireHandler) Unpublish(c *gin.Context) {
	qCode := c.Param("code")
	if qCode == "" {
		h.ErrorResponse(c, errors.WithCode(code.ErrQuestionnaireInvalidInput, "问卷编码不能为空"))
		return
	}

	result, err := h.lifecycleService.Unpublish(c.Request.Context(), qCode)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, response.NewQuestionnaireResponseFromResult(result))
}

// Archive 归档问卷
// @Summary 归档问卷
// @Description 归档不再使用的问卷
// @Tags Questionnaire-Lifecycle
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "问卷编码"
// @Success 200 {object} handler.Response{data=response.QuestionnaireResponse}
// @Router /api/v1/questionnaires/{code}/archive [post]
func (h *QuestionnaireHandler) Archive(c *gin.Context) {
	qCode := c.Param("code")
	if qCode == "" {
		h.ErrorResponse(c, errors.WithCode(code.ErrQuestionnaireInvalidInput, "问卷编码不能为空"))
		return
	}

	result, err := h.lifecycleService.Archive(c.Request.Context(), qCode)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, response.NewQuestionnaireResponseFromResult(result))
}

// Delete 删除问卷
// @Summary 删除问卷
// @Description 删除草稿状态的问卷
// @Tags Questionnaire-Lifecycle
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "问卷编码"
// @Success 200 {object} handler.Response
// @Router /api/v1/questionnaires/{code} [delete]
func (h *QuestionnaireHandler) Delete(c *gin.Context) {
	qCode := c.Param("code")
	if qCode == "" {
		h.ErrorResponse(c, errors.WithCode(code.ErrQuestionnaireInvalidInput, "问卷编码不能为空"))
		return
	}

	err := h.lifecycleService.Delete(c.Request.Context(), qCode)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, gin.H{"message": "删除成功"})
}

// ============= Content API (内容编辑) =============

// AddQuestion 添加问题
// @Summary 添加问题
// @Description 向问卷添加新问题
// @Tags Questionnaire-Content
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "问卷编码"
// @Param request body request.AddQuestionRequest true "添加问题请求"
// @Success 200 {object} handler.Response{data=response.QuestionnaireResponse}
// @Router /api/v1/questionnaires/{code}/questions [post]
func (h *QuestionnaireHandler) AddQuestion(c *gin.Context) {
	qCode := c.Param("code")
	if qCode == "" {
		h.ErrorResponse(c, errors.WithCode(code.ErrQuestionnaireInvalidInput, "问卷编码不能为空"))
		return
	}

	var req request.AddQuestionRequest
	if err := h.BindJSON(c, &req); err != nil {
		h.ErrorResponse(c, err)
		return
	}

	// 转换 viewmodel.OptionDTO 为 questionnaire.OptionDTO
	options := make([]questionnaire.OptionDTO, 0, len(req.Options))
	for _, opt := range req.Options {
		options = append(options, questionnaire.OptionDTO{
			Label: opt.Content,
			Value: opt.Code,
			Score: int(opt.Score),
		})
	}

	dto := questionnaire.AddQuestionDTO{
		QuestionnaireCode: qCode,
		Code:              req.Code,
		Stem:              req.Stem,
		Type:              req.Type,
		Options:           options,
		Required:          req.Required,
		Description:       req.Description,
	}

	result, err := h.contentService.AddQuestion(c.Request.Context(), dto)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, response.NewQuestionnaireResponseFromResult(result))
}

// UpdateQuestion 更新问题
// @Summary 更新问题
// @Description 更新问卷中的某个问题
// @Tags Questionnaire-Content
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "问卷编码"
// @Param questionCode path string true "问题编码"
// @Param request body request.UpdateQuestionRequest true "更新问题请求"
// @Success 200 {object} handler.Response{data=response.QuestionnaireResponse}
// @Router /api/v1/questionnaires/{code}/questions/{questionCode} [put]
func (h *QuestionnaireHandler) UpdateQuestion(c *gin.Context) {
	qCode := c.Param("code")
	questionCode := c.Param("questionCode")
	if qCode == "" || questionCode == "" {
		h.ErrorResponse(c, errors.WithCode(code.ErrQuestionnaireInvalidInput, "问卷编码和问题编码不能为空"))
		return
	}

	var req request.UpdateQuestionRequest
	if err := h.BindJSON(c, &req); err != nil {
		h.ErrorResponse(c, err)
		return
	}

	// 转换 viewmodel.OptionDTO 为 questionnaire.OptionDTO
	options := make([]questionnaire.OptionDTO, 0, len(req.Options))
	for _, opt := range req.Options {
		options = append(options, questionnaire.OptionDTO{
			Label: opt.Content,
			Value: opt.Code,
			Score: int(opt.Score),
		})
	}

	dto := questionnaire.UpdateQuestionDTO{
		QuestionnaireCode: qCode,
		Code:              req.Code,
		Stem:              req.Stem,
		Type:              req.Type,
		Options:           options,
		Required:          req.Required,
		Description:       req.Description,
	}

	result, err := h.contentService.UpdateQuestion(c.Request.Context(), dto)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, response.NewQuestionnaireResponseFromResult(result))
}

// RemoveQuestion 删除问题
// @Summary 删除问题
// @Description 从问卷中删除某个问题
// @Tags Questionnaire-Content
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "问卷编码"
// @Param questionCode path string true "问题编码"
// @Success 200 {object} handler.Response{data=response.QuestionnaireResponse}
// @Router /api/v1/questionnaires/{code}/questions/{questionCode} [delete]
func (h *QuestionnaireHandler) RemoveQuestion(c *gin.Context) {
	qCode := c.Param("code")
	questionCode := c.Param("questionCode")
	if qCode == "" || questionCode == "" {
		h.ErrorResponse(c, errors.WithCode(code.ErrQuestionnaireInvalidInput, "问卷编码和问题编码不能为空"))
		return
	}

	result, err := h.contentService.RemoveQuestion(c.Request.Context(), qCode, questionCode)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, response.NewQuestionnaireResponseFromResult(result))
}

// ReorderQuestions 重排问题顺序
// @Summary 重排问题顺序
// @Description 调整问卷中问题的显示顺序
// @Tags Questionnaire-Content
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "问卷编码"
// @Param request body request.ReorderQuestionsRequest true "重排问题请求"
// @Success 200 {object} handler.Response{data=response.QuestionnaireResponse}
// @Router /api/v1/questionnaires/{code}/questions/reorder [post]
func (h *QuestionnaireHandler) ReorderQuestions(c *gin.Context) {
	qCode := c.Param("code")
	if qCode == "" {
		h.ErrorResponse(c, errors.WithCode(code.ErrQuestionnaireInvalidInput, "问卷编码不能为空"))
		return
	}

	var req request.ReorderQuestionsRequest
	if err := h.BindJSON(c, &req); err != nil {
		h.ErrorResponse(c, err)
		return
	}

	result, err := h.contentService.ReorderQuestions(c.Request.Context(), qCode, req.QuestionCodes)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, response.NewQuestionnaireResponseFromResult(result))
}

// BatchUpdateQuestions 批量更新问题
// @Summary 批量更新问题
// @Description 批量更新问卷的所有问题（前端保存时使用）
// @Tags Questionnaire-Content
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "问卷编码"
// @Param request body request.BatchUpdateQuestionsRequest true "批量更新请求"
// @Success 200 {object} handler.Response{data=response.QuestionnaireResponse}
// @Router /api/v1/questionnaires/{code}/questions/batch [put]
func (h *QuestionnaireHandler) BatchUpdateQuestions(c *gin.Context) {
	qCode := c.Param("code")
	if qCode == "" {
		h.ErrorResponse(c, errors.WithCode(code.ErrQuestionnaireInvalidInput, "问卷编码不能为空"))
		return
	}

	var req request.BatchUpdateQuestionsRequest
	if err := h.BindJSON(c, &req); err != nil {
		h.ErrorResponse(c, err)
		return
	}

	// 转换 viewmodel.QuestionDTO 为 questionnaire.QuestionDTO
	questions := make([]questionnaire.QuestionDTO, 0, len(req.Questions))
	for _, q := range req.Questions {
		options := make([]questionnaire.OptionDTO, 0, len(q.Options))
		for _, opt := range q.Options {
			options = append(options, questionnaire.OptionDTO{
				Label: opt.Content,
				Value: opt.Code,
				Score: int(opt.Score),
			})
		}

		questions = append(questions, questionnaire.QuestionDTO{
			Code:        q.Code,
			Stem:        q.Stem,
			Type:        q.Type,
			Options:     options,
			Required:    false, // viewmodel.QuestionDTO 没有 Required 字段
			Description: q.Tips,
		})
	}

	result, err := h.contentService.BatchUpdateQuestions(c.Request.Context(), qCode, questions)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, response.NewQuestionnaireResponseFromResult(result))
}

// ============= Query API (查询) =============

// GetByCode 根据编码获取问卷
// @Summary 获取问卷详情
// @Description 根据编码获取问卷的完整信息（管理端使用）
// @Tags Questionnaire-Query
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "问卷编码"
// @Success 200 {object} handler.Response{data=response.QuestionnaireResponse}
// @Router /api/v1/questionnaires/{code} [get]
func (h *QuestionnaireHandler) GetByCode(c *gin.Context) {
	qCode := c.Param("code")
	if qCode == "" {
		h.ErrorResponse(c, errors.WithCode(code.ErrQuestionnaireInvalidInput, "问卷编码不能为空"))
		return
	}

	result, err := h.queryService.GetByCode(c.Request.Context(), qCode)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, response.NewQuestionnaireResponseFromResult(result))
}

// List 查询问卷列表
// @Summary 查询问卷列表
// @Description 分页查询问卷列表，支持条件筛选（管理端使用）
// @Tags Questionnaire-Query
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(10)
// @Param status query string false "状态筛选"
// @Param title query string false "标题筛选"
// @Success 200 {object} handler.Response{data=response.QuestionnaireListResponse}
// @Router /api/v1/questionnaires [get]
func (h *QuestionnaireHandler) List(c *gin.Context) {
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page <= 0 {
		h.ErrorResponse(c, errors.WithCode(code.ErrQuestionnaireInvalidInput, "页码必须为正整数"))
		return
	}

	pageSize, err := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	if err != nil || pageSize <= 0 || pageSize > 100 {
		h.ErrorResponse(c, errors.WithCode(code.ErrQuestionnaireInvalidInput, "每页数量必须为1-100的整数"))
		return
	}

	conditions := make(map[string]interface{})
	if status := c.Query("status"); status != "" {
		conditions["status"] = status
	}
	if title := c.Query("title"); title != "" {
		conditions["title"] = title
	}

	dto := questionnaire.ListQuestionnairesDTO{
		Page:       page,
		PageSize:   pageSize,
		Conditions: conditions,
	}

	result, err := h.queryService.List(c.Request.Context(), dto)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, response.NewQuestionnaireListResponseFromResult(result))
}

// GetPublishedByCode 获取已发布问卷
// @Summary 获取已发布问卷
// @Description 根据编码获取已发布的问卷（C端答题使用）
// @Tags Questionnaire-Query
// @Accept json
// @Produce json
// @Param code path string true "问卷编码"
// @Success 200 {object} handler.Response{data=response.QuestionnaireResponse}
// @Router /api/v1/public/questionnaires/{code} [get]
func (h *QuestionnaireHandler) GetPublishedByCode(c *gin.Context) {
	qCode := c.Param("code")
	if qCode == "" {
		h.ErrorResponse(c, errors.WithCode(code.ErrQuestionnaireInvalidInput, "问卷编码不能为空"))
		return
	}

	result, err := h.queryService.GetPublishedByCode(c.Request.Context(), qCode)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, response.NewQuestionnaireResponseFromResult(result))
}

// ListPublished 查询已发布问卷列表
// @Summary 查询已发布问卷列表
// @Description 分页查询已发布的问卷列表（C端答题使用）
// @Tags Questionnaire-Query
// @Accept json
// @Produce json
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(10)
// @Success 200 {object} handler.Response{data=response.QuestionnaireListResponse}
// @Router /api/v1/public/questionnaires [get]
func (h *QuestionnaireHandler) ListPublished(c *gin.Context) {
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page <= 0 {
		h.ErrorResponse(c, errors.WithCode(code.ErrQuestionnaireInvalidInput, "页码必须为正整数"))
		return
	}

	pageSize, err := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	if err != nil || pageSize <= 0 || pageSize > 100 {
		h.ErrorResponse(c, errors.WithCode(code.ErrQuestionnaireInvalidInput, "每页数量必须为1-100的整数"))
		return
	}

	dto := questionnaire.ListQuestionnairesDTO{
		Page:       page,
		PageSize:   pageSize,
		Conditions: make(map[string]interface{}),
	}

	result, err := h.queryService.ListPublished(c.Request.Context(), dto)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, response.NewQuestionnaireListResponseFromResult(result))
}
