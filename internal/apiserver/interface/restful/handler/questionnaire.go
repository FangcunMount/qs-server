package handler

import (
	"github.com/asaskevich/govalidator"
	"github.com/gin-gonic/gin"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire/port"
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
	var req port.QuestionnaireCreateRequest
	if err := h.BindJSON(c, &req); err != nil {
		h.ErrorResponse(c, err)
		return
	}
	if ok, err := govalidator.ValidateStruct(req); !ok {
		h.ErrorResponse(c, err)
		return
	}

	questionnaire, err := h.questionnaireCreator.CreateQuestionnaire(c, req)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, questionnaire)
}

// EditQuestionnaire 编辑问卷
func (h *QuestionnaireHandler) EditQuestionnaire(c *gin.Context) {
	var req port.QuestionnaireEditRequest
	if err := h.BindQuery(c, &req); err != nil {
		h.ErrorResponse(c, err)
		return
	}

	questionnaire, err := h.questionnaireEditor.EditBasicInfo(c, req)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, questionnaire)
}

// PublishQuestionnaire 发布问卷
func (h *QuestionnaireHandler) PublishQuestionnaire(c *gin.Context) {
	var req port.QuestionnairePublishRequest
	if err := h.BindQuery(c, &req); err != nil {
		h.ErrorResponse(c, err)
		return
	}

	questionnaire, err := h.questionnairePublisher.PublishQuestionnaire(c, req)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, questionnaire)
}

// UnpublishQuestionnaire 下架问卷
func (h *QuestionnaireHandler) UnpublishQuestionnaire(c *gin.Context) {
	var req port.QuestionnaireUnpublishRequest
	if err := h.BindQuery(c, &req); err != nil {
		h.ErrorResponse(c, err)
		return
	}

	questionnaire, err := h.questionnairePublisher.UnpublishQuestionnaire(c, req)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, questionnaire)
}

// GetQuestionnaire 获取问卷
func (h *QuestionnaireHandler) GetQuestionnaire(c *gin.Context) {
	var req port.QuestionnaireIDRequest
	if err := h.BindQuery(c, &req); err != nil {
		h.ErrorResponse(c, err)
		return
	}

	questionnaire, err := h.questionnaireQueryer.GetQuestionnaire(c, req)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, questionnaire)
}
