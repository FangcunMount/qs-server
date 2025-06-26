package questionnaire

import (
	"github.com/gin-gonic/gin"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/adapters/api/http/handlers"
	questionnaireApp "github.com/yshujie/questionnaire-scale/internal/apiserver/application/questionnaire"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/questionnaire/commands"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/questionnaire/queries"
)

// Handler 问卷HTTP处理器
type Handler struct {
	*handlers.BaseHandler
	questionnaireService *questionnaireApp.Service
}

// NewHandler 创建问卷处理器
func NewHandler(questionnaireService *questionnaireApp.Service) handlers.Handler {
	return &Handler{
		BaseHandler:          handlers.NewBaseHandler(),
		questionnaireService: questionnaireService,
	}
}

// GetName 获取Handler名称
func (h *Handler) GetName() string {
	return "questionnaire"
}

// 路由注册已移至 internal/apiserver/routers.go 进行集中管理

// CreateQuestionnaire 创建问卷
// @Summary 创建问卷
// @Description 创建新的问卷
// @Tags questionnaires
// @Accept json
// @Produce json
// @Param questionnaire body commands.CreateQuestionnaireCommand true "问卷信息"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/questionnaires [post]
func (h *Handler) CreateQuestionnaire(c *gin.Context) {
	var cmd commands.CreateQuestionnaireCommand
	if err := h.BindJSON(c, &cmd); err != nil {
		return // 错误响应已在BindJSON中处理
	}

	questionnaire, err := h.questionnaireService.CreateQuestionnaire(c.Request.Context(), cmd)
	if err != nil {
		h.InternalErrorResponse(c, "创建问卷失败", err)
		return
	}

	h.SuccessResponseWithMessage(c, "创建成功", h.questionnaireToResponse(questionnaire))
}

// GetQuestionnaire 获取问卷
// @Summary 获取问卷详情
// @Description 根据ID或代码获取问卷详情
// @Tags questionnaires
// @Accept json
// @Produce json
// @Param id query string false "问卷ID"
// @Param code query string false "问卷代码"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/questionnaires [get]
func (h *Handler) GetQuestionnaire(c *gin.Context) {
	var query queries.GetQuestionnaireQuery
	if err := h.BindQuery(c, &query); err != nil {
		return
	}

	questionnaire, err := h.questionnaireService.GetQuestionnaire(c.Request.Context(), query)
	if err != nil {
		h.NotFoundResponse(c, "问卷不存在", err)
		return
	}

	h.SuccessResponseWithMessage(c, "获取成功", h.questionnaireToResponse(questionnaire))
}

// ListQuestionnaires 获取问卷列表
// @Summary 获取问卷列表
// @Description 分页获取问卷列表
// @Tags questionnaires
// @Accept json
// @Produce json
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Param creator_id query string false "创建者ID"
// @Param status query int false "状态"
// @Param keyword query string false "关键字"
// @Param sort_by query string false "排序字段"
// @Param sort_order query string false "排序方式" Enums(asc, desc)
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/questionnaires/list [get]
func (h *Handler) ListQuestionnaires(c *gin.Context) {
	var query queries.ListQuestionnairesQuery
	if err := h.BindQuery(c, &query); err != nil {
		return
	}

	result, err := h.questionnaireService.ListQuestionnaires(c.Request.Context(), query)
	if err != nil {
		h.InternalErrorResponse(c, "获取列表失败", err)
		return
	}

	// 转换为响应格式
	items := make([]map[string]interface{}, len(result.Items))
	for i, questionnaire := range result.Items {
		items[i] = h.questionnaireToResponse(questionnaire)
	}

	data := gin.H{
		"items":      items,
		"pagination": result.Pagination,
	}

	h.SuccessResponseWithMessage(c, "获取成功", data)
}

// UpdateQuestionnaire 更新问卷
// @Summary 更新问卷
// @Description 更新问卷基础信息
// @Tags questionnaires
// @Accept json
// @Produce json
// @Param id path string true "问卷ID"
// @Param questionnaire body commands.UpdateQuestionnaireCommand true "问卷更新信息"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/questionnaires/{id} [put]
func (h *Handler) UpdateQuestionnaire(c *gin.Context) {
	var cmd commands.UpdateQuestionnaireCommand
	if err := h.BindJSON(c, &cmd); err != nil {
		return
	}

	// 从路径参数获取ID
	cmd.ID = h.GetPathParam(c, "id")

	questionnaire, err := h.questionnaireService.UpdateQuestionnaire(c.Request.Context(), cmd)
	if err != nil {
		h.InternalErrorResponse(c, "更新问卷失败", err)
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
	cmd := commands.PublishQuestionnaireCommand{
		ID: h.GetPathParam(c, "id"),
	}

	err := h.questionnaireService.PublishQuestionnaire(c.Request.Context(), cmd)
	if err != nil {
		h.InternalErrorResponse(c, "发布问卷失败", err)
		return
	}

	h.SuccessResponseWithMessage(c, "发布成功", nil)
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
	cmd := commands.DeleteQuestionnaireCommand{
		ID: h.GetPathParam(c, "id"),
	}

	err := h.questionnaireService.DeleteQuestionnaire(c.Request.Context(), cmd)
	if err != nil {
		h.InternalErrorResponse(c, "删除问卷失败", err)
		return
	}

	h.SuccessResponseWithMessage(c, "删除成功", nil)
}

// CheckDataConsistency 检查数据一致性
// @Summary 检查问卷数据一致性
// @Description 检查MySQL和MongoDB中问卷数据的一致性
// @Tags questionnaires
// @Accept json
// @Produce json
// @Param id path string true "问卷ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/questionnaires/{id}/consistency [get]
func (h *Handler) CheckDataConsistency(c *gin.Context) {
	id := h.GetPathParam(c, "id")

	result, err := h.questionnaireService.CheckQuestionnaireDataConsistency(c.Request.Context(), id)
	if err != nil {
		h.InternalErrorResponse(c, "数据一致性检查失败", err)
		return
	}

	h.SuccessResponseWithMessage(c, "检查完成", result)
}

// RepairData 修复数据不一致
// @Summary 修复问卷数据不一致
// @Description 修复MySQL和MongoDB中问卷数据的不一致
// @Tags questionnaires
// @Accept json
// @Produce json
// @Param id path string true "问卷ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/questionnaires/{id}/repair [post]
func (h *Handler) RepairData(c *gin.Context) {
	id := h.GetPathParam(c, "id")

	err := h.questionnaireService.RepairQuestionnaireData(c.Request.Context(), id)
	if err != nil {
		h.InternalErrorResponse(c, "数据修复失败", err)
		return
	}

	h.SuccessResponseWithMessage(c, "数据修复成功", nil)
}

// 辅助方法：将DTO转换为响应格式
func (h *Handler) questionnaireToResponse(q interface{}) map[string]interface{} {
	// TODO: 实现具体的转换逻辑
	// 这里需要将DTO转换为适合HTTP响应的格式
	return map[string]interface{}{
		"message": "questionnaire response conversion not implemented yet",
		"data":    q,
	}
}
