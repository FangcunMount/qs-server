package handler

import (
	"strconv"

	"github.com/asaskevich/govalidator"
	"github.com/gin-gonic/gin"
	"github.com/fangcun-mount/qs-server/internal/apiserver/application/dto"
	"github.com/fangcun-mount/qs-server/internal/apiserver/domain/questionnaire/port"
	"github.com/fangcun-mount/qs-server/internal/apiserver/interface/restful/mapper"
	"github.com/fangcun-mount/qs-server/internal/apiserver/interface/restful/request"
	"github.com/fangcun-mount/qs-server/internal/apiserver/interface/restful/response"
	"github.com/fangcun-mount/qs-server/internal/pkg/code"
	"github.com/fangcun-mount/qs-server/pkg/errors"
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

	// 转换为 DTO
	questionnaireDTO := &dto.QuestionnaireDTO{
		Title:       req.Title,
		Description: req.Description,
		ImgUrl:      req.ImgUrl,
	}

	// 调用领域服务
	result, err := h.questionnaireCreator.CreateQuestionnaire(c, questionnaireDTO)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, response.NewQuestionnaireResponse(result))
}

// EditBasicInfo 编辑问卷基本信息
func (h *QuestionnaireHandler) EditBasicInfo(c *gin.Context) {
	// 从路径参数获取code
	qCode := c.Param("code")
	if qCode == "" {
		h.ErrorResponse(c, errors.WithCode(code.ErrQuestionnaireInvalidInput, "问卷代码不能为空"))
		return
	}

	var req request.EditQuestionnaireBasicInfoRequest
	if err := h.BindJSON(c, &req); err != nil {
		h.ErrorResponse(c, err)
		return
	}

	// 转换为 DTO
	questionnaireDTO := &dto.QuestionnaireDTO{
		Code:        qCode,
		Title:       req.Title,
		Description: req.Description,
		ImgUrl:      req.ImgUrl,
	}

	// 调用领域服务
	result, err := h.questionnaireEditor.EditBasicInfo(c, questionnaireDTO)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, response.NewQuestionnaireResponse(result))
}

// UpdateQuestions 更新问卷的问题列表
func (h *QuestionnaireHandler) UpdateQuestions(c *gin.Context) {
	// 从路径参数获取code
	qCode := c.Param("code")
	if qCode == "" {
		h.ErrorResponse(c, errors.WithCode(code.ErrQuestionnaireInvalidInput, "问卷代码不能为空"))
		return
	}

	var req request.EditQuestionnaireQuestionsRequest
	if err := h.BindJSON(c, &req); err != nil {
		h.ErrorResponse(c, err)
		return
	}

	// 转换为 DTO
	questions := mapper.NewQuestionMapper().ToDTOs(req.Questions)

	// 调用领域服务
	result, err := h.questionnaireEditor.UpdateQuestions(c, qCode, questions)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, response.NewQuestionnaireResponse(result))
}

// PublishQuestionnaire 发布问卷
func (h *QuestionnaireHandler) PublishQuestionnaire(c *gin.Context) {
	// 从路径参数获取code
	qCode := c.Param("code")
	if qCode == "" {
		h.ErrorResponse(c, errors.WithCode(code.ErrQuestionnaireInvalidInput, "问卷代码不能为空"))
		return
	}

	// 调用领域服务
	result, err := h.questionnairePublisher.Publish(c, qCode)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, response.NewQuestionnaireResponse(result))
}

// UnpublishQuestionnaire 下架问卷
func (h *QuestionnaireHandler) UnpublishQuestionnaire(c *gin.Context) {
	// 从路径参数获取code
	qCode := c.Param("code")
	if qCode == "" {
		h.ErrorResponse(c, errors.WithCode(code.ErrQuestionnaireInvalidInput, "问卷代码不能为空"))
		return
	}

	// 调用领域服务
	result, err := h.questionnairePublisher.Unpublish(c, qCode)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, response.NewQuestionnaireResponse(result))
}

// QueryOne 查询单个问卷
func (h *QuestionnaireHandler) QueryOne(c *gin.Context) {
	// 从路径参数获取code
	qCode := c.Param("code")
	if qCode == "" {
		h.ErrorResponse(c, errors.WithCode(code.ErrQuestionnaireInvalidInput, "问卷代码不能为空"))
		return
	}

	// 调用领域服务
	result, err := h.questionnaireQueryer.GetQuestionnaireByCode(c, qCode)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, response.NewQuestionnaireResponse(result))
}

// QueryList 查询问卷列表
func (h *QuestionnaireHandler) QueryList(c *gin.Context) {
	// 获取分页参数
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

	// 获取查询条件
	conditions := make(map[string]string)
	if status := c.Query("status"); status != "" {
		conditions["status"] = status
	}
	if title := c.Query("title"); title != "" {
		conditions["title"] = title
	}

	// 调用领域服务
	questionnaires, total, err := h.questionnaireQueryer.ListQuestionnaires(c, page, pageSize, conditions)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, response.NewQuestionnaireListResponse(questionnaires, total, page, pageSize))
}
