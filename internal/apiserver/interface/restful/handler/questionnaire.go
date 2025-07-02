package handler

import (
	"github.com/asaskevich/govalidator"
	"github.com/gin-gonic/gin"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire/port"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/interface/restful/dto"
)

// QuestionnaireHandler 问卷处理器
type QuestionnaireHandler struct {
	BaseHandler
	questionnaireCreator   port.QuestionnaireCreator
	questionnaireEditor    port.QuestionnaireEditor
	questionnairePublisher port.QuestionnairePublisher
	questionnaireQueryer   port.QuestionnaireQueryer
}

// NewQuestionnaireHandler 创建问卷处理器
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
	var req dto.CreateQuestionnaireRequest
	if err := h.BindJSON(c, &req); err != nil {
		h.ErrorResponse(c, err)
		return
	}
	if ok, err := govalidator.ValidateStruct(req); !ok {
		h.ErrorResponse(c, err)
		return
	}

	// 调用领域服务
	q, err := h.questionnaireCreator.CreateQuestionnaire(c, req.Title, req.Description, req.ImgUrl)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	// 转换为DTO响应
	response := &dto.QuestionnaireBasicInfoResponse{
		Code:        q.GetCode().Value(),
		Title:       q.GetTitle(),
		Description: q.GetDescription(),
		ImgUrl:      q.GetImgUrl(),
		Version:     q.GetVersion().Value(),
		Status:      q.GetStatus().Value(),
	}

	h.SuccessResponse(c, response)
}

// EditQuestionnaire 编辑问卷
func (h *QuestionnaireHandler) EditBasicInfo(c *gin.Context) {
	var req dto.EditQuestionnaireBasicInfoRequest
	if err := h.BindQuery(c, &req); err != nil {
		h.ErrorResponse(c, err)
		return
	}

	// 调用领域服务
	q, err := h.questionnaireEditor.EditBasicInfo(
		c,
		questionnaire.NewQuestionnaireCode(req.Code),
		req.Title,
		req.Description,
		req.ImgUrl,
	)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	// 转换为DTO响应
	response := &dto.QuestionnaireBasicInfoResponse{
		Code:        q.GetCode().Value(),
		Title:       q.GetTitle(),
		Description: q.GetDescription(),
		ImgUrl:      q.GetImgUrl(),
		Version:     q.GetVersion().Value(),
		Status:      q.GetStatus().Value(),
	}

	h.SuccessResponse(c, response)
}

// PublishQuestionnaire 发布问卷
func (h *QuestionnaireHandler) PublishQuestionnaire(c *gin.Context) {
	var req dto.PublishQuestionnaireRequest
	if err := h.BindQuery(c, &req); err != nil {
		h.ErrorResponse(c, err)
		return
	}

	// 调用领域服务
	q, err := h.questionnairePublisher.Publish(c, questionnaire.NewQuestionnaireCode(req.Code))
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	// 转换为DTO响应
	response := &dto.QuestionnaireBasicInfoResponse{
		Code:        q.GetCode().Value(),
		Title:       q.GetTitle(),
		Description: q.GetDescription(),
		ImgUrl:      q.GetImgUrl(),
		Version:     q.GetVersion().Value(),
		Status:      q.GetStatus().Value(),
	}

	h.SuccessResponse(c, response)
}

// UnpublishQuestionnaire 下架问卷
func (h *QuestionnaireHandler) UnpublishQuestionnaire(c *gin.Context) {
	var req dto.UnpublishQuestionnaireRequest
	if err := h.BindQuery(c, &req); err != nil {
		h.ErrorResponse(c, err)
		return
	}

	// 调用领域服务
	q, err := h.questionnairePublisher.Unpublish(c, questionnaire.NewQuestionnaireCode(req.Code))
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	// 转换为DTO响应
	response := &dto.QuestionnaireBasicInfoResponse{
		Code:        q.GetCode().Value(),
		Title:       q.GetTitle(),
		Description: q.GetDescription(),
		ImgUrl:      q.GetImgUrl(),
		Version:     q.GetVersion().Value(),
		Status:      q.GetStatus().Value(),
	}

	h.SuccessResponse(c, response)
}

// GetQuestionnaire 获取问卷
func (h *QuestionnaireHandler) QueryOne(c *gin.Context) {
	var req dto.QueryQuestionnaireRequest
	if err := h.BindQuery(c, &req); err != nil {
		h.ErrorResponse(c, err)
		return
	}

	// 调用领域服务
	q, err := h.questionnaireQueryer.GetQuestionnaireByCode(c, req.Code)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	// 转换为DTO响应
	response := &dto.QuestionnaireBasicInfoResponse{
		Code:        q.GetCode().Value(),
		Title:       q.GetTitle(),
		Description: q.GetDescription(),
		ImgUrl:      q.GetImgUrl(),
		Version:     q.GetVersion().Value(),
		Status:      q.GetStatus().Value(),
	}

	h.SuccessResponse(c, response)
}

func (h *QuestionnaireHandler) QueryList(c *gin.Context) {
	var req dto.QueryQuestionnaireListRequest
	if err := h.BindQuery(c, &req); err != nil {
		h.ErrorResponse(c, err)
		return
	}
}
