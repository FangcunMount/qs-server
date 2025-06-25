package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/services"
)

// QuestionnaireHandler 问卷HTTP处理器
type QuestionnaireHandler struct {
	questionnaireService *services.QuestionnaireService
}

// NewQuestionnaireHandler 创建问卷处理器
func NewQuestionnaireHandler(questionnaireService *services.QuestionnaireService) *QuestionnaireHandler {
	return &QuestionnaireHandler{
		questionnaireService: questionnaireService,
	}
}

// CreateQuestionnaire 创建问卷
// @Summary 创建问卷
// @Description 创建新的问卷
// @Tags questionnaires
// @Accept json
// @Produce json
// @Param questionnaire body services.CreateQuestionnaireCommand true "问卷信息"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/questionnaires [post]
func (h *QuestionnaireHandler) CreateQuestionnaire(c *gin.Context) {
	var cmd services.CreateQuestionnaireCommand
	if err := c.ShouldBindJSON(&cmd); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "参数错误",
			"error":   err.Error(),
		})
		return
	}

	questionnaire, err := h.questionnaireService.CreateQuestionnaire(c.Request.Context(), cmd)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "创建问卷失败",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "创建成功",
		"data":    h.questionnaireToResponse(questionnaire),
	})
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
func (h *QuestionnaireHandler) GetQuestionnaire(c *gin.Context) {
	var query services.GetQuestionnaireQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "参数错误",
			"error":   err.Error(),
		})
		return
	}

	questionnaire, err := h.questionnaireService.GetQuestionnaire(c.Request.Context(), query)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": "问卷不存在",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "获取成功",
		"data":    h.questionnaireToResponse(questionnaire),
	})
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
func (h *QuestionnaireHandler) ListQuestionnaires(c *gin.Context) {
	var query services.ListQuestionnairesQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "参数错误",
			"error":   err.Error(),
		})
		return
	}

	result, err := h.questionnaireService.ListQuestionnaires(c.Request.Context(), query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "获取列表失败",
			"error":   err.Error(),
		})
		return
	}

	// 转换为响应格式
	items := make([]map[string]interface{}, len(result.Items))
	for i, questionnaire := range result.Items {
		items[i] = h.questionnaireToResponse(questionnaire)
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "获取成功",
		"data": gin.H{
			"items":       items,
			"total_count": result.TotalCount,
			"has_more":    result.HasMore,
			"page":        result.Page,
			"page_size":   result.PageSize,
		},
	})
}

// UpdateQuestionnaire 更新问卷
// @Summary 更新问卷
// @Description 更新问卷基础信息
// @Tags questionnaires
// @Accept json
// @Produce json
// @Param id path string true "问卷ID"
// @Param questionnaire body services.UpdateQuestionnaireCommand true "问卷更新信息"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/questionnaires/{id} [put]
func (h *QuestionnaireHandler) UpdateQuestionnaire(c *gin.Context) {
	var cmd services.UpdateQuestionnaireCommand
	if err := c.ShouldBindJSON(&cmd); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "参数错误",
			"error":   err.Error(),
		})
		return
	}

	// 从路径参数获取ID
	cmd.ID = c.Param("id")

	questionnaire, err := h.questionnaireService.UpdateQuestionnaire(c.Request.Context(), cmd)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "更新问卷失败",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "更新成功",
		"data":    h.questionnaireToResponse(questionnaire),
	})
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
func (h *QuestionnaireHandler) PublishQuestionnaire(c *gin.Context) {
	cmd := services.PublishQuestionnaireCommand{
		ID: c.Param("id"),
	}

	err := h.questionnaireService.PublishQuestionnaire(c.Request.Context(), cmd)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "发布问卷失败",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "发布成功",
	})
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
func (h *QuestionnaireHandler) DeleteQuestionnaire(c *gin.Context) {
	cmd := services.DeleteQuestionnaireCommand{
		ID: c.Param("id"),
	}

	err := h.questionnaireService.DeleteQuestionnaire(c.Request.Context(), cmd)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "删除问卷失败",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "删除成功",
	})
}

// 辅助方法：将领域对象转换为响应格式
func (h *QuestionnaireHandler) questionnaireToResponse(q interface{}) map[string]interface{} {
	// TODO: 实现具体的转换逻辑
	// 这里需要将领域对象转换为适合HTTP响应的格式
	return map[string]interface{}{
		"message": "questionnaire response conversion not implemented yet",
	}
}
