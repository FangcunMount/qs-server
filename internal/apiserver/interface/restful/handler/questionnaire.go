package handler

import (
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/gin-gonic/gin"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire/port"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/interface/restful/dto"
)

type QuestionnaireHandler struct {
	BaseHandler
	questionnaireCreator   port.QuestionnaireCreator
	questionnaireEditor    port.QuestionnaireEditor
	questionnairePublisher port.QuestionnairePublisher
	questionnaireQueryer   port.QuestionnaireQueryer
}

func NewQuestionnaireHandler(
	questionnaireCreator port.QuestionnaireCreator,
	questionnaireEditor port.QuestionnaireEditor,
	questionnairePublisher port.QuestionnairePublisher,
	questionnaireQueryer port.QuestionnaireQueryer,
) *QuestionnaireHandler {
	return &QuestionnaireHandler{
		questionnaireCreator:   questionnaireCreator,
		questionnaireEditor:    questionnaireEditor,
		questionnairePublisher: questionnairePublisher,
		questionnaireQueryer:   questionnaireQueryer,
	}
}

// CreateQuestionnaire 创建问卷
func (h *QuestionnaireHandler) CreateQuestionnaire(c *gin.Context) {
	var req dto.QuestionnaireCreateRequest
	if err := h.BindJSON(c, &req); err != nil {
		h.ErrorResponse(c, err)
		return
	}
	if ok, err := govalidator.ValidateStruct(req); !ok {
		h.ErrorResponse(c, err)
		return
	}

	// 调用领域服务
	questionnaire, err := h.questionnaireCreator.CreateQuestionnaire(c, req.Title, req.Description, req.ImgUrl)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	// 转换为DTO响应
	response := &dto.QuestionnaireResponse{
		ID:          questionnaire.ID.Value(),
		Code:        questionnaire.Code,
		Title:       questionnaire.Title,
		Description: questionnaire.Description,
		ImgUrl:      questionnaire.ImgUrl,
		Version:     questionnaire.Version,
		Status:      questionnaire.Status,
		CreatedAt:   questionnaire.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   questionnaire.UpdatedAt.Format(time.RFC3339),
	}

	h.SuccessResponse(c, response)
}

// EditQuestionnaire 编辑问卷
func (h *QuestionnaireHandler) EditQuestionnaire(c *gin.Context) {
	var req dto.QuestionnaireEditRequest
	if err := h.BindQuery(c, &req); err != nil {
		h.ErrorResponse(c, err)
		return
	}

	// 调用领域服务
	questionnaire, err := h.questionnaireEditor.EditBasicInfo(c, req.ID, req.Title, req.ImgUrl, req.Version)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	// 转换为DTO响应
	response := &dto.QuestionnaireResponse{
		ID:          questionnaire.ID.Value(),
		Code:        questionnaire.Code,
		Title:       questionnaire.Title,
		Description: questionnaire.Description,
		ImgUrl:      questionnaire.ImgUrl,
		Version:     questionnaire.Version,
		Status:      questionnaire.Status,
		CreatedAt:   questionnaire.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   questionnaire.UpdatedAt.Format(time.RFC3339),
	}

	h.SuccessResponse(c, response)
}

// PublishQuestionnaire 发布问卷
func (h *QuestionnaireHandler) PublishQuestionnaire(c *gin.Context) {
	var req dto.QuestionnairePublishRequest
	if err := h.BindQuery(c, &req); err != nil {
		h.ErrorResponse(c, err)
		return
	}

	// 调用领域服务
	questionnaire, err := h.questionnairePublisher.PublishQuestionnaire(c, req.ID)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	// 转换为DTO响应
	response := &dto.QuestionnaireResponse{
		ID:          questionnaire.ID.Value(),
		Code:        questionnaire.Code,
		Title:       questionnaire.Title,
		Description: questionnaire.Description,
		ImgUrl:      questionnaire.ImgUrl,
		Version:     questionnaire.Version,
		Status:      questionnaire.Status,
		CreatedAt:   questionnaire.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   questionnaire.UpdatedAt.Format(time.RFC3339),
	}

	h.SuccessResponse(c, response)
}

// UnpublishQuestionnaire 下架问卷
func (h *QuestionnaireHandler) UnpublishQuestionnaire(c *gin.Context) {
	var req dto.QuestionnaireUnpublishRequest
	if err := h.BindQuery(c, &req); err != nil {
		h.ErrorResponse(c, err)
		return
	}

	// 调用领域服务
	questionnaire, err := h.questionnairePublisher.UnpublishQuestionnaire(c, req.ID)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	// 转换为DTO响应
	response := &dto.QuestionnaireResponse{
		ID:          questionnaire.ID.Value(),
		Code:        questionnaire.Code,
		Title:       questionnaire.Title,
		Description: questionnaire.Description,
		ImgUrl:      questionnaire.ImgUrl,
		Version:     questionnaire.Version,
		Status:      questionnaire.Status,
		CreatedAt:   questionnaire.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   questionnaire.UpdatedAt.Format(time.RFC3339),
	}

	h.SuccessResponse(c, response)
}

// GetQuestionnaire 获取问卷
func (h *QuestionnaireHandler) GetQuestionnaire(c *gin.Context) {
	var req dto.QuestionnaireIDRequest
	if err := h.BindQuery(c, &req); err != nil {
		h.ErrorResponse(c, err)
		return
	}

	// 调用领域服务
	questionnaire, err := h.questionnaireQueryer.GetQuestionnaire(c, req.ID)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	// 转换为DTO响应
	response := &dto.QuestionnaireResponse{
		ID:          questionnaire.ID.Value(),
		Code:        questionnaire.Code,
		Title:       questionnaire.Title,
		Description: questionnaire.Description,
		ImgUrl:      questionnaire.ImgUrl,
		Version:     questionnaire.Version,
		Status:      questionnaire.Status,
		CreatedAt:   questionnaire.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   questionnaire.UpdatedAt.Format(time.RFC3339),
	}

	h.SuccessResponse(c, response)
}
