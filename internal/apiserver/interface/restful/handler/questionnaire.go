package handler

import (
	"github.com/asaskevich/govalidator"
	"github.com/gin-gonic/gin"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire/port"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/interface/restful/mapper"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/interface/restful/request"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/interface/restful/response"
	"github.com/yshujie/questionnaire-scale/internal/pkg/code"
	"github.com/yshujie/questionnaire-scale/pkg/errors"
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
	var req request.CreateQuestionnaireRequest
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
	response := &response.QuestionnaireBasicInfoResponse{
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
	// 从路径参数获取code
	qCode := c.Param("code")
	if qCode == "" {
		h.ErrorResponse(c, errors.WithCode(code.ErrValidation, "问卷代码不能为空"))
		return
	}

	var req request.EditQuestionnaireBasicInfoRequest
	if err := h.BindJSON(c, &req); err != nil {
		h.ErrorResponse(c, err)
		return
	}

	// 调用领域服务
	q, err := h.questionnaireEditor.EditBasicInfo(
		c,
		questionnaire.NewQuestionnaireCode(qCode),
		req.Title,
		req.Description,
		req.ImgUrl,
	)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	// 转换为DTO响应
	response := &response.QuestionnaireBasicInfoResponse{
		Code:        q.GetCode().Value(),
		Title:       q.GetTitle(),
		Description: q.GetDescription(),
		ImgUrl:      q.GetImgUrl(),
		Version:     q.GetVersion().Value(),
		Status:      q.GetStatus().Value(),
	}

	h.SuccessResponse(c, response)
}

// UpdateQuestions 更新问卷的问题列表
func (h *QuestionnaireHandler) UpdateQuestions(c *gin.Context) {
	// 从路径参数获取code
	qCode := c.Param("code")
	if qCode == "" {
		h.ErrorResponse(c, errors.WithCode(code.ErrValidation, "问卷代码不能为空"))
		return
	}

	var req request.EditQuestionnaireQuestionsRequest
	if err := h.BindJSON(c, &req); err != nil {
		h.ErrorResponse(c, err)
		return
	}

	// 调用领域服务
	q, err := h.questionnaireEditor.UpdateQuestions(
		c,
		questionnaire.NewQuestionnaireCode(qCode),
		mapper.NewQuestionMapper().MapQuestionsToBOs(req.Questions),
	)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	// 转换为DTO响应
	response := &response.QuestionnaireQuestionsResponse{
		Code:      q.GetCode().Value(),
		Questions: mapper.NewQuestionMapper().MapQuestionsToDTOs(q.GetQuestions()),
	}

	h.SuccessResponse(c, response)
}

// PublishQuestionnaire 发布问卷
func (h *QuestionnaireHandler) PublishQuestionnaire(c *gin.Context) {
	// 从路径参数获取code
	qCode := c.Param("code")
	if qCode == "" {
		h.ErrorResponse(c, errors.WithCode(code.ErrValidation, "问卷代码不能为空"))
		return
	}

	// 调用领域服务
	q, err := h.questionnairePublisher.Publish(c, questionnaire.NewQuestionnaireCode(qCode))
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	// 转换为DTO响应
	response := &response.QuestionnaireBasicInfoResponse{
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
	// 从路径参数获取code
	qCode := c.Param("code")
	if qCode == "" {
		h.ErrorResponse(c, errors.WithCode(code.ErrValidation, "问卷代码不能为空"))
		return
	}

	// 调用领域服务
	q, err := h.questionnairePublisher.Unpublish(c, questionnaire.NewQuestionnaireCode(qCode))
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	// 转换为DTO响应
	response := &response.QuestionnaireBasicInfoResponse{
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
	// 从路径参数获取code
	qCode := c.Param("code")
	if qCode == "" {
		h.ErrorResponse(c, errors.WithCode(code.ErrValidation, "问卷代码不能为空"))
		return
	}

	// 调用领域服务
	q, err := h.questionnaireQueryer.GetQuestionnaireByCode(c, qCode)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	// 转换为DTO响应
	response := &response.QuestionnaireResponse{
		Questionnaire: response.QuestionnaireBasicInfoResponse{
			Code:        q.GetCode().Value(),
			Title:       q.GetTitle(),
			Description: q.GetDescription(),
			ImgUrl:      q.GetImgUrl(),
			Version:     q.GetVersion().Value(),
			Status:      q.GetStatus().Value(),
		},
		Questions: mapper.NewQuestionMapper().MapQuestionsToDTOs(q.GetQuestions()),
	}

	h.SuccessResponse(c, response)
}

func (h *QuestionnaireHandler) QueryList(c *gin.Context) {
	var req request.QueryQuestionnaireListRequest
	if err := h.BindQuery(c, &req); err != nil {
		h.ErrorResponse(c, err)
		return
	}

	// 调用领域服务
	questionnaires, total, err := h.questionnaireQueryer.ListQuestionnaires(c, req.Page, req.PageSize, req.Conditions)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	// 转换为DTO响应
	questionnaireDTOs := make([]response.QuestionnaireBasicInfoResponse, len(questionnaires))
	for i, q := range questionnaires {
		questionnaireDTOs[i] = response.QuestionnaireBasicInfoResponse{
			Code:        q.GetCode().Value(),
			Title:       q.GetTitle(),
			Description: q.GetDescription(),
			ImgUrl:      q.GetImgUrl(),
			Version:     q.GetVersion().Value(),
			Status:      q.GetStatus().Value(),
		}
	}
	response := &response.QuestionnaireListResponse{
		Questionnaires: questionnaireDTOs,
		TotalCount:     total,
		Page:           req.Page,
		PageSize:       req.PageSize,
	}

	h.SuccessResponse(c, response)
}
