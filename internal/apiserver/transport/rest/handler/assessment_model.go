package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/request"
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/response"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/pkg/core"
	"github.com/asaskevich/govalidator"
	"github.com/gin-gonic/gin"
)

type AssessmentModelHandler struct {
	BaseHandler
	service modelcatalog.Service
}

func NewAssessmentModelHandler(service modelcatalog.Service) *AssessmentModelHandler {
	return &AssessmentModelHandler{service: service}
}

// List 获取测评模型列表
// @Summary 获取测评模型列表
// @Tags AssessmentModel
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param kind query string false "模型类型"
// @Param sub_kind query string false "子类型"
// @Param status query string false "状态"
// @Param keyword query string false "关键词"
// @Param category query string false "分类"
// @Param algorithm query string false "算法"
// @Param page query int false "页码"
// @Param page_size query int false "每页数量"
// @Success 200 {object} core.Response{data=response.AssessmentModelListResponse}
// @Router /api/v1/assessment-models [get]
func (h *AssessmentModelHandler) List(c *gin.Context) {
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page <= 0 {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "页码无效"))
		return
	}
	pageSize, err := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if err != nil || pageSize <= 0 {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "每页数量无效"))
		return
	}
	result, err := h.service.List(c.Request.Context(), modelcatalog.ListModelsDTO{
		Kind:      c.Query("kind"),
		SubKind:   c.Query("sub_kind"),
		Status:    c.Query("status"),
		Keyword:   c.Query("keyword"),
		Category:  c.Query("category"),
		Algorithm: c.Query("algorithm"),
		Page:      page,
		PageSize:  pageSize,
	})
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, (*response.AssessmentModelListResponse)(result))
}

// Create 创建测评模型
// @Summary 创建测评模型
// @Tags AssessmentModel
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param request body request.CreateAssessmentModelRequest true "创建测评模型请求"
// @Success 200 {object} core.Response{data=response.AssessmentModelResponse}
// @Router /api/v1/assessment-models [post]
func (h *AssessmentModelHandler) Create(c *gin.Context) {
	var req request.CreateAssessmentModelRequest
	if err := h.bindAndValidate(c, &req); err != nil {
		h.Error(c, err)
		return
	}
	result, err := h.service.Create(c.Request.Context(), modelcatalog.CreateModelDTO{
		Code:                 req.Code,
		Kind:                 req.Kind,
		SubKind:              req.SubKind,
		Algorithm:            req.Algorithm,
		ProductChannel:       req.ProductChannel,
		Title:                req.Title,
		Description:          req.Description,
		Category:             req.Category,
		Tags:                 req.Tags,
		QuestionnaireCode:    req.QuestionnaireCode,
		QuestionnaireVersion: req.QuestionnaireVersion,
	})
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, (*response.AssessmentModelResponse)(result))
}

// Get 获取测评模型详情
// @Summary 获取测评模型详情
// @Tags AssessmentModel
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "模型编码"
// @Success 200 {object} core.Response{data=response.AssessmentModelResponse}
// @Router /api/v1/assessment-models/{code} [get]
func (h *AssessmentModelHandler) Get(c *gin.Context) {
	result, err := h.service.Get(c.Request.Context(), h.modelCode(c))
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, (*response.AssessmentModelResponse)(result))
}

func (h *AssessmentModelHandler) UpdateBasicInfo(c *gin.Context) {
	var req request.UpdateAssessmentModelBasicInfoRequest
	if err := h.bindAndValidate(c, &req); err != nil {
		h.Error(c, err)
		return
	}
	result, err := h.service.UpdateBasicInfo(c.Request.Context(), modelcatalog.UpdateBasicInfoDTO{
		Code:           h.modelCode(c),
		Title:          req.Title,
		Description:    req.Description,
		SubKind:        req.SubKind,
		Algorithm:      req.Algorithm,
		ProductChannel: req.ProductChannel,
		Category:       req.Category,
		Tags:           req.Tags,
	})
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, (*response.AssessmentModelResponse)(result))
}

func (h *AssessmentModelHandler) Delete(c *gin.Context) {
	if err := h.service.Delete(c.Request.Context(), h.modelCode(c)); err != nil {
		h.Error(c, err)
		return
	}
	h.SuccessResponseWithMessage(c, "删除成功", nil)
}

func (h *AssessmentModelHandler) Publish(c *gin.Context) {
	result, err := h.service.Validate(c.Request.Context(), h.modelCode(c))
	if err != nil {
		h.Error(c, err)
		return
	}
	if result != nil && !result.Passed {
		c.AbortWithStatusJSON(http.StatusBadRequest, core.Response{
			Code:    code.ErrAssessmentModelValidationFailed,
			Message: "模型校验失败",
			Data:    (*response.AssessmentModelValidationResponse)(result),
		})
		return
	}
	h.transition(c, h.service.Publish)
}

func (h *AssessmentModelHandler) Unpublish(c *gin.Context) {
	h.transition(c, h.service.Unpublish)
}

func (h *AssessmentModelHandler) Archive(c *gin.Context) {
	h.transition(c, h.service.Archive)
}

// BindQuestionnaire 绑定问卷
// @Summary 绑定问卷
// @Tags AssessmentModel
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "模型编码"
// @Param request body request.BindAssessmentModelQuestionnaireRequest true "绑定问卷请求"
// @Success 200 {object} core.Response{data=response.AssessmentModelQuestionnaireResponse}
// @Router /api/v1/assessment-models/{code}/questionnaire [put]
func (h *AssessmentModelHandler) BindQuestionnaire(c *gin.Context) {
	var req request.BindAssessmentModelQuestionnaireRequest
	if err := h.bindAndValidate(c, &req); err != nil {
		h.Error(c, err)
		return
	}
	result, err := h.service.BindQuestionnaire(c.Request.Context(), modelcatalog.BindQuestionnaireDTO{
		Code:                 h.modelCode(c),
		QuestionnaireCode:    req.QuestionnaireCode,
		QuestionnaireVersion: req.QuestionnaireVersion,
	})
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, (*response.AssessmentModelQuestionnaireResponse)(result))
}

// GetQuestionnaire 获取测评模型绑定的问卷
// @Summary 获取测评模型绑定的问卷
// @Tags AssessmentModel
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "模型编码"
// @Success 200 {object} core.Response{data=response.AssessmentModelQuestionnaireResponse}
// @Router /api/v1/assessment-models/{code}/questionnaire [get]
func (h *AssessmentModelHandler) GetQuestionnaire(c *gin.Context) {
	result, err := h.service.GetQuestionnaire(c.Request.Context(), h.modelCode(c))
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, (*response.AssessmentModelQuestionnaireResponse)(result))
}

// GetDefinition 获取测评模型定义
// @Summary 获取测评模型定义
// @Tags AssessmentModel
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "模型编码"
// @Success 200 {object} core.Response
// @Router /api/v1/assessment-models/{code}/definition [get]
func (h *AssessmentModelHandler) GetDefinition(c *gin.Context) {
	result, err := h.service.GetDefinition(c.Request.Context(), h.modelCode(c))
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, (*response.AssessmentModelDefinitionResponse)(result))
}

// UpdateDefinition 更新测评模型定义
// @Summary 更新测评模型定义
// @Tags AssessmentModel
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "模型编码"
// @Param request body request.UpdateAssessmentModelDefinitionRequest true "更新定义请求"
// @Success 200 {object} core.Response
// @Router /api/v1/assessment-models/{code}/definition [put]
func (h *AssessmentModelHandler) UpdateDefinition(c *gin.Context) {
	var req request.UpdateAssessmentModelDefinitionRequest
	if err := h.bindAndValidate(c, &req); err != nil {
		h.Error(c, err)
		return
	}
	result, err := h.service.UpdateDefinition(c.Request.Context(), h.modelCode(c), modelcatalog.DefinitionDTO{
		Kind:          req.Kind,
		SubKind:       req.SubKind,
		Algorithm:     req.Algorithm,
		PayloadFormat: req.PayloadFormat,
		Payload:       req.Payload,
	})
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, (*response.AssessmentModelDefinitionResponse)(result))
}

// Options 获取测评模型选项
// @Summary 获取测评模型选项
// @Tags AssessmentModel
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param kind query string false "模型类型"
// @Success 200 {object} core.Response{data=response.AssessmentModelOptionsResponse}
// @Router /api/v1/assessment-models/options [get]
func (h *AssessmentModelHandler) Options(c *gin.Context) {
	result, err := h.service.Options(c.Request.Context(), c.Query("kind"))
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, (*response.AssessmentModelOptionsResponse)(result))
}

// ApplyCodes 申请编码
// @Summary 申请编码
// @Tags AssessmentModel
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "模型编码"
// @Param request body request.ApplyAssessmentModelCodesRequest true "申请编码请求"
// @Success 200 {object} core.Response{data=response.AssessmentModelCodesResponse}
// @Router /api/v1/assessment-models/{code}/codes/apply [post]
func (h *AssessmentModelHandler) ApplyCodes(c *gin.Context) {
	var req request.ApplyAssessmentModelCodesRequest
	if err := h.bindAndValidate(c, &req); err != nil {
		h.Error(c, err)
		return
	}
	codes, err := h.service.ApplyCodes(c.Request.Context(), modelcatalog.ApplyCodesDTO{
		Code:   h.modelCode(c),
		Target: req.Target,
		Count:  req.Count,
	})
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, response.AssessmentModelCodesResponse{Codes: codes})
}

func (h *AssessmentModelHandler) Validate(c *gin.Context) {
	result, err := h.service.Validate(c.Request.Context(), h.modelCode(c))
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, (*response.AssessmentModelValidationResponse)(result))
}

// PreviewReport 预览报告
// @Summary 预览报告
// @Tags AssessmentModel
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "模型编码"
// @Param request body request.PreviewAssessmentModelReportRequest true "预览报告请求"
// @Success 200 {object} core.Response{data=response.AssessmentModelPreviewReportResponse}
// @Router /api/v1/assessment-models/{code}/preview-report [post]
func (h *AssessmentModelHandler) PreviewReport(c *gin.Context) {
	var req request.PreviewAssessmentModelReportRequest
	if err := h.BindJSON(c, &req); err != nil {
		h.Error(c, err)
		return
	}
	payload, _ := json.Marshal(req)
	result, err := h.service.PreviewReport(c.Request.Context(), h.modelCode(c), payload)
	if err != nil {
		if vf, ok := modelcatalog.ValidationFailedFrom(err); ok {
			c.AbortWithStatusJSON(http.StatusBadRequest, core.Response{
				Code:    code.ErrAssessmentModelValidationFailed,
				Message: "模型校验失败",
				Data:    (*response.AssessmentModelValidationResponse)(vf.Result),
			})
			return
		}
		h.Error(c, err)
		return
	}
	h.Success(c, (*response.AssessmentModelPreviewReportResponse)(result))
}

func (h *AssessmentModelHandler) GetQRCode(c *gin.Context) {
	qrCodeURL, err := h.service.GetQRCode(c.Request.Context(), h.modelCode(c))
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, response.NewQRCodeResponse(qrCodeURL))
}

func (h *AssessmentModelHandler) transition(c *gin.Context, action func(context.Context, string) (*modelcatalog.ModelSummary, error)) {
	result, err := action(c.Request.Context(), h.modelCode(c))
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, (*response.AssessmentModelResponse)(result))
}

func (h *AssessmentModelHandler) bindAndValidate(c *gin.Context, req interface{}) error {
	if err := h.BindJSON(c, req); err != nil {
		return err
	}
	if ok, err := govalidator.ValidateStruct(req); !ok {
		return err
	}
	return nil
}

func (h *AssessmentModelHandler) modelCode(c *gin.Context) string {
	return c.Param("code")
}
