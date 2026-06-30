package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/assessmentmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/request"
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/response"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/pkg/core"
	"github.com/asaskevich/govalidator"
	"github.com/gin-gonic/gin"
)

type AssessmentModelHandler struct {
	BaseHandler
	service assessmentmodel.Service
}

func NewAssessmentModelHandler(service assessmentmodel.Service) *AssessmentModelHandler {
	return &AssessmentModelHandler{service: service}
}

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
	result, err := h.service.List(c.Request.Context(), assessmentmodel.ListModelsDTO{
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

func (h *AssessmentModelHandler) Create(c *gin.Context) {
	var req request.CreateAssessmentModelRequest
	if err := h.bindAndValidate(c, &req); err != nil {
		h.Error(c, err)
		return
	}
	result, err := h.service.Create(c.Request.Context(), assessmentmodel.CreateModelDTO{
		Code:                 req.Code,
		Kind:                 req.Kind,
		SubKind:              req.SubKind,
		Algorithm:            req.Algorithm,
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
	result, err := h.service.UpdateBasicInfo(c.Request.Context(), assessmentmodel.UpdateBasicInfoDTO{
		Code:        h.modelCode(c),
		Title:       req.Title,
		Description: req.Description,
		SubKind:     req.SubKind,
		Algorithm:   req.Algorithm,
		Category:    req.Category,
		Tags:        req.Tags,
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

func (h *AssessmentModelHandler) BindQuestionnaire(c *gin.Context) {
	var req request.BindAssessmentModelQuestionnaireRequest
	if err := h.bindAndValidate(c, &req); err != nil {
		h.Error(c, err)
		return
	}
	result, err := h.service.BindQuestionnaire(c.Request.Context(), assessmentmodel.BindQuestionnaireDTO{
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

func (h *AssessmentModelHandler) GetQuestionnaire(c *gin.Context) {
	result, err := h.service.GetQuestionnaire(c.Request.Context(), h.modelCode(c))
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, (*response.AssessmentModelQuestionnaireResponse)(result))
}

func (h *AssessmentModelHandler) GetDefinition(c *gin.Context) {
	result, err := h.service.GetDefinition(c.Request.Context(), h.modelCode(c))
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, (*response.AssessmentModelDefinitionResponse)(result))
}

func (h *AssessmentModelHandler) UpdateDefinition(c *gin.Context) {
	var req request.UpdateAssessmentModelDefinitionRequest
	if err := h.bindAndValidate(c, &req); err != nil {
		h.Error(c, err)
		return
	}
	result, err := h.service.UpdateDefinition(c.Request.Context(), h.modelCode(c), assessmentmodel.DefinitionDTO{
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

func (h *AssessmentModelHandler) Options(c *gin.Context) {
	result, err := h.service.Options(c.Request.Context(), c.Query("kind"))
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, (*response.AssessmentModelOptionsResponse)(result))
}

func (h *AssessmentModelHandler) ApplyCodes(c *gin.Context) {
	var req request.ApplyAssessmentModelCodesRequest
	if err := h.bindAndValidate(c, &req); err != nil {
		h.Error(c, err)
		return
	}
	codes, err := h.service.ApplyCodes(c.Request.Context(), assessmentmodel.ApplyCodesDTO{
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

func (h *AssessmentModelHandler) PreviewReport(c *gin.Context) {
	var req request.PreviewAssessmentModelReportRequest
	if err := h.BindJSON(c, &req); err != nil {
		h.Error(c, err)
		return
	}
	payload, _ := json.Marshal(req)
	result, err := h.service.PreviewReport(c.Request.Context(), h.modelCode(c), payload)
	if err != nil {
		if vf, ok := assessmentmodel.ValidationFailedFrom(err); ok {
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

func (h *AssessmentModelHandler) transition(c *gin.Context, action func(context.Context, string) (*assessmentmodel.ModelSummary, error)) {
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
