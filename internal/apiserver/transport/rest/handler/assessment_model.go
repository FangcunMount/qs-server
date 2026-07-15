package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	assessmentassets "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/assets"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/request"
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/response"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/pkg/core"
	"github.com/asaskevich/govalidator"
	"github.com/gin-gonic/gin"
)

// AssessmentModelHandler is the sole management transport for every
// assessment-model kind. It deliberately knows no family-specific command or
// snapshot DTO.
type AssessmentModelHandler struct {
	BaseHandler
	management  modelcatalog.CatalogManagementService
	definition  modelcatalog.DefinitionAuthoringService
	publication modelcatalog.PublicationService
	query       modelcatalog.CatalogQueryService
	assets      modelcatalog.AssessmentImageService
}

func NewAssessmentModelHandler(
	management modelcatalog.CatalogManagementService,
	definition modelcatalog.DefinitionAuthoringService,
	publication modelcatalog.PublicationService,
	query modelcatalog.CatalogQueryService,
	assets ...modelcatalog.AssessmentImageService,
) *AssessmentModelHandler {
	var imageService modelcatalog.AssessmentImageService
	if len(assets) > 0 {
		imageService = assets[0]
	}
	return &AssessmentModelHandler{management: management, definition: definition, publication: publication, query: query, assets: imageService}
}

// List lists draft catalogue records. Use the published endpoints for
// immutable execution records.
// @Summary 获取测评模型列表
// @Description 统一目录返回 canonical kind；人格模型使用 typology，personality 仅为读兼容别名。
// @Tags AssessmentModel
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param kind query string false "模型类型"
// @Param status query string false "状态"
// @Param product_channel query string false "产品通道"
// @Param questionnaire_code query string false "问卷编码"
// @Param questionnaire_version query string false "问卷版本"
// @Param page query int false "页码"
// @Param page_size query int false "每页数量"
// @Success 200 {object} core.Response{data=response.AssessmentModelListResponse}
// @Router /api/v1/assessment-models [get]
func (h *AssessmentModelHandler) List(c *gin.Context) {
	input, err := modelListInput(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	actor, err := assessmentModelActorContext(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	result, err := h.query.List(c.Request.Context(), actor, input)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, (*response.AssessmentModelListResponse)(result))
}

// Create creates a model aggregate. Scale-specific catalogue attributes are
// ordinary AssessmentModel metadata and are accepted only with kind=scale.
// @Summary 创建测评模型
// @Tags AssessmentModel
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param request body request.CreateAssessmentModelRequest true "创建请求"
// @Success 200 {object} core.Response{data=response.AssessmentModelResponse}
// @Router /api/v1/assessment-models [post]
func (h *AssessmentModelHandler) Create(c *gin.Context) {
	var req request.CreateAssessmentModelRequest
	if err := h.bindAndValidate(c, &req); err != nil {
		h.Error(c, err)
		return
	}
	actor, err := assessmentModelActorContext(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	result, err := h.management.Create(c.Request.Context(), actor, modelcatalog.CreateModelDTO{
		Code: req.Code, Kind: req.Kind, SubKind: req.SubKind, Algorithm: req.Algorithm, ProductChannel: req.ProductChannel,
		Title: req.Title, Description: req.Description, Category: req.Category, Stages: req.Stages, ApplicableAges: req.ApplicableAges,
		Reporters: req.Reporters, Tags: req.Tags, QuestionnaireCode: req.QuestionnaireCode, QuestionnaireVersion: req.QuestionnaireVersion,
	})
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, (*response.AssessmentModelResponse)(result))
}

// Get returns the mutable catalogue aggregate summary.
// @Summary 获取测评模型详情
// @Tags AssessmentModel
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "模型编码"
// @Success 200 {object} core.Response{data=response.AssessmentModelResponse}
// @Router /api/v1/assessment-models/{code} [get]
func (h *AssessmentModelHandler) Get(c *gin.Context) {
	actor, err := assessmentModelActorContext(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	result, err := h.query.Get(c.Request.Context(), actor, h.modelCode(c))
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, (*response.AssessmentModelResponse)(result))
}

// UpdateBasicInfo updates generic model metadata.
// @Summary 更新测评模型基本信息
// @Tags AssessmentModel
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "模型编码"
// @Param request body request.UpdateAssessmentModelBasicInfoRequest true "更新请求"
// @Success 200 {object} core.Response{data=response.AssessmentModelResponse}
// @Router /api/v1/assessment-models/{code}/basic-info [put]
func (h *AssessmentModelHandler) UpdateBasicInfo(c *gin.Context) {
	var req request.UpdateAssessmentModelBasicInfoRequest
	if err := h.bindAndValidate(c, &req); err != nil {
		h.Error(c, err)
		return
	}
	actor, err := assessmentModelActorContext(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	result, err := h.management.UpdateBasicInfo(c.Request.Context(), actor, modelcatalog.UpdateBasicInfoDTO{
		Code: h.modelCode(c), Title: req.Title, Description: req.Description, SubKind: req.SubKind, Algorithm: req.Algorithm,
		ProductChannel: req.ProductChannel, Category: req.Category, Stages: req.Stages, ApplicableAges: req.ApplicableAges,
		Reporters: req.Reporters, Tags: req.Tags,
	})
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, (*response.AssessmentModelResponse)(result))
}

// RestoreDraftFromPublished restores a mutable draft for a legacy orphaned
// published snapshot. The operator must still make any edit and publish it via
// the normal assessment release workflow.
// @Summary 从发布快照恢复测评草稿
// @Tags AssessmentModel
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "模型编码"
// @Success 200 {object} core.Response{data=response.AssessmentModelResponse}
// @Router /api/v1/assessment-models/{code}/restore-draft [post]
func (h *AssessmentModelHandler) RestoreDraftFromPublished(c *gin.Context) {
	actor, err := assessmentModelActorContext(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	result, err := h.management.RestoreDraftFromPublished(c.Request.Context(), actor, h.modelCode(c))
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, (*response.AssessmentModelResponse)(result))
}

// @Summary 删除已归档测评模型
// @Tags AssessmentModel
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "模型编码"
// @Success 200 {object} core.Response
// @Router /api/v1/assessment-models/{code} [delete]
func (h *AssessmentModelHandler) Delete(c *gin.Context) {
	actor, err := assessmentModelActorContext(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	if err := h.management.Delete(c.Request.Context(), actor, h.modelCode(c)); err != nil {
		h.Error(c, err)
		return
	}
	h.SuccessResponseWithMessage(c, "删除成功", nil)
}

// @Summary 发布测评模型
// @Tags AssessmentModel
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "模型编码"
// @Success 200 {object} core.Response{data=response.AssessmentModelResponse}
// @Router /api/v1/assessment-models/{code}/publish [post]
func (h *AssessmentModelHandler) Publish(c *gin.Context) {
	actor, err := assessmentModelActorContext(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	result, err := h.publication.Publish(c.Request.Context(), actor, h.modelCode(c))
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, (*response.AssessmentModelResponse)(result))
}

// @Summary 下架测评模型
// @Tags AssessmentModel
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "模型编码"
// @Success 200 {object} core.Response{data=response.AssessmentModelResponse}
// @Router /api/v1/assessment-models/{code}/unpublish [post]
func (h *AssessmentModelHandler) Unpublish(c *gin.Context) {
	actor, err := assessmentModelActorContext(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	result, err := h.publication.Unpublish(c.Request.Context(), actor, h.modelCode(c))
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, (*response.AssessmentModelResponse)(result))
}

// @Summary 归档测评模型
// @Tags AssessmentModel
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "模型编码"
// @Success 200 {object} core.Response{data=response.AssessmentModelResponse}
// @Router /api/v1/assessment-models/{code}/archive [post]
func (h *AssessmentModelHandler) Archive(c *gin.Context) {
	actor, err := assessmentModelActorContext(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	result, err := h.management.Archive(c.Request.Context(), actor, h.modelCode(c))
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, (*response.AssessmentModelResponse)(result))
}

// BindQuestionnaire binds a draft model to a questionnaire version.
// @Summary 绑定问卷
// @Tags AssessmentModel
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "模型编码"
// @Param request body request.BindAssessmentModelQuestionnaireRequest true "绑定请求"
// @Success 200 {object} core.Response{data=response.AssessmentModelQuestionnaireResponse}
// @Router /api/v1/assessment-models/{code}/questionnaire [put]
func (h *AssessmentModelHandler) BindQuestionnaire(c *gin.Context) {
	var req request.BindAssessmentModelQuestionnaireRequest
	if err := h.bindAndValidate(c, &req); err != nil {
		h.Error(c, err)
		return
	}
	actor, err := assessmentModelActorContext(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	result, err := h.management.BindQuestionnaire(c.Request.Context(), actor, modelcatalog.BindQuestionnaireDTO{Code: h.modelCode(c), QuestionnaireCode: req.QuestionnaireCode, QuestionnaireVersion: req.QuestionnaireVersion})
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, (*response.AssessmentModelQuestionnaireResponse)(result))
}

// @Summary 获取模型问卷绑定
// @Tags AssessmentModel
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "模型编码"
// @Success 200 {object} core.Response{data=response.AssessmentModelQuestionnaireResponse}
// @Router /api/v1/assessment-models/{code}/questionnaire [get]
func (h *AssessmentModelHandler) GetQuestionnaire(c *gin.Context) {
	actor, err := assessmentModelActorContext(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	result, err := h.query.GetQuestionnaire(c.Request.Context(), actor, h.modelCode(c))
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, (*response.AssessmentModelQuestionnaireResponse)(result))
}

// GetDefinition returns canonical DefinitionV2.
// @Summary 获取测评模型定义
// @Tags AssessmentModel
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "模型编码"
// @Success 200 {object} core.Response{data=response.DefinitionV2Wire}
// @Router /api/v1/assessment-models/{code}/definition [get]
func (h *AssessmentModelHandler) GetDefinition(c *gin.Context) {
	actor, err := assessmentModelActorContext(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	result, err := h.definition.GetDefinition(c.Request.Context(), actor, h.modelCode(c))
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, (*response.AssessmentModelDefinitionResponse)(result))
}

// UpdateDefinition replaces the complete canonical DefinitionV2.
// @Summary 更新测评模型定义
// @Tags AssessmentModel
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "模型编码"
// @Param request body response.DefinitionV2Wire true "DefinitionV2"
// @Success 200 {object} core.Response{data=response.DefinitionV2Wire}
// @Router /api/v1/assessment-models/{code}/definition [put]
func (h *AssessmentModelHandler) UpdateDefinition(c *gin.Context) {
	var req request.UpdateAssessmentModelDefinitionRequest
	if err := h.bindAndValidate(c, &req); err != nil {
		h.Error(c, err)
		return
	}
	actor, err := assessmentModelActorContext(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	definition := domain.Definition(req)
	result, err := h.definition.SaveDefinition(c.Request.Context(), actor, h.modelCode(c), &definition)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, (*response.AssessmentModelDefinitionResponse)(result))
}

// UploadMBTIOutcomeImage uploads one immutable MBTI outcome portrait. The
// returned URL is intentionally not persisted until the editor saves DefinitionV2.
// @Summary 上传 MBTI 结果人物图片
// @Tags AssessmentModel
// @Accept mpfd
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "模型编码"
// @Param outcome_code path string true "MBTI 结果编码"
// @Param file formData file true "PNG/JPEG/WebP 图片，最大 5 MiB"
// @Success 200 {object} core.Response{data=response.AssessmentModelImageUploadResponse}
// @Router /api/v1/assessment-models/{code}/outcomes/{outcome_code}/image [post]
func (h *AssessmentModelHandler) UploadMBTIOutcomeImage(c *gin.Context) {
	if h.assets == nil || h.assets.MaxUploadBytes() <= 0 {
		h.Error(c, errors.WithCode(code.ErrInternalServerError, "assessment image assets are not configured"))
		return
	}
	maxBytes := h.assets.MaxUploadBytes()
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes+1024)
	fileHeader, err := c.FormFile("file")
	if err != nil {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "image file is required"))
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "open image file: %v", err))
		return
	}
	defer file.Close()
	content, err := assessmentassets.ReadAllLimited(file, maxBytes)
	if err != nil {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "read image file: %v", err))
		return
	}
	actor, err := assessmentModelActorContext(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	result, err := h.assets.UploadMBTIOutcomeImage(c.Request.Context(), actor, modelcatalog.AssessmentImageUploadInput{
		ModelCode: h.modelCode(c), OutcomeCode: c.Param("outcome_code"), Filename: fileHeader.Filename, Content: content,
	})
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, (*response.AssessmentModelImageUploadResponse)(result))
}

// Options exposes presentation metadata for a model kind.
// @Summary 获取测评模型选项
// @Tags AssessmentModel
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param kind query string false "模型类型"
// @Success 200 {object} core.Response{data=response.AssessmentModelOptionsResponse}
// @Router /api/v1/assessment-models/options [get]
func (h *AssessmentModelHandler) Options(c *gin.Context) {
	actor, err := assessmentModelActorContext(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	result, err := h.query.Options(c.Request.Context(), actor, c.Query("kind"))
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, (*response.AssessmentModelOptionsResponse)(result))
}

// @Summary 申请模型定义编码
// @Tags AssessmentModel
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "模型编码"
// @Param request body request.ApplyAssessmentModelCodesRequest true "编码申请"
// @Success 200 {object} core.Response{data=response.AssessmentModelCodesResponse}
// @Router /api/v1/assessment-models/{code}/codes/apply [post]
func (h *AssessmentModelHandler) ApplyCodes(c *gin.Context) {
	var req request.ApplyAssessmentModelCodesRequest
	if err := h.bindAndValidate(c, &req); err != nil {
		h.Error(c, err)
		return
	}
	actor, err := assessmentModelActorContext(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	values, err := h.definition.ApplyCodes(c.Request.Context(), actor, modelcatalog.ApplyCodesDTO{Code: h.modelCode(c), Target: req.Target, Count: req.Count})
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, response.AssessmentModelCodesResponse{Codes: values})
}

// @Summary 校验测评模型定义
// @Tags AssessmentModel
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "模型编码"
// @Success 200 {object} core.Response{data=response.AssessmentModelValidationResponse}
// @Router /api/v1/assessment-models/{code}/validate [post]
func (h *AssessmentModelHandler) Validate(c *gin.Context) {
	actor, err := assessmentModelActorContext(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	result, err := h.definition.ValidateDefinition(c.Request.Context(), actor, h.modelCode(c))
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, (*response.AssessmentModelValidationResponse)(result))
}

// @Summary 预览测评模型报告
// @Tags AssessmentModel
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "模型编码"
// @Param request body response.PreviewReportRequestWire true "预览输入"
// @Success 200 {object} core.Response{data=response.PreviewReportWire}
// @Router /api/v1/assessment-models/{code}/preview-report [post]
func (h *AssessmentModelHandler) PreviewReport(c *gin.Context) {
	var req request.PreviewAssessmentModelReportRequest
	if err := h.BindJSON(c, &req); err != nil {
		h.Error(c, err)
		return
	}
	payload, _ := json.Marshal(req)
	actor, err := assessmentModelActorContext(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	result, err := h.definition.PreviewReport(c.Request.Context(), actor, h.modelCode(c), payload)
	if err != nil {
		if vf, ok := modelcatalog.ValidationFailedFrom(err); ok {
			c.AbortWithStatusJSON(http.StatusBadRequest, core.Response{Code: code.ErrAssessmentModelValidationFailed, Message: "模型校验失败", Data: (*response.AssessmentModelValidationResponse)(vf.Result)})
			return
		}
		h.Error(c, err)
		return
	}
	h.Success(c, (*response.AssessmentModelPreviewReportResponse)(result))
}

// @Summary 获取测评模型二维码
// @Tags AssessmentModel
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "模型编码"
// @Success 200 {object} core.Response{data=response.QRCodeResponse}
// @Router /api/v1/assessment-models/{code}/qrcode [get]
func (h *AssessmentModelHandler) GetQRCode(c *gin.Context) {
	actor, err := assessmentModelActorContext(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	url, err := h.query.GetQRCode(c.Request.Context(), actor, h.modelCode(c))
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, response.NewQRCodeResponse(url))
}

// GetPublished returns an immutable published model and its canonical
// DefinitionV2. It is an admin read contract, not an execution resolver.
// @Summary 获取已发布测评模型
// @Tags AssessmentModel
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "模型编码"
// @Param version query string false "发布版本"
// @Success 200 {object} core.Response{data=response.PublishedAssessmentModelResponse}
// @Router /api/v1/assessment-models/published/{code} [get]
func (h *AssessmentModelHandler) GetPublished(c *gin.Context) {
	actor, err := assessmentModelActorContext(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	result, err := h.query.GetPublished(c.Request.Context(), actor, h.modelCode(c), c.Query("version"))
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, (*response.PublishedAssessmentModelResponse)(result))
}

// @Summary 获取已发布测评模型列表
// @Tags AssessmentModel
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param kind query string false "模型类型"
// @Param questionnaire_code query string false "问卷编码"
// @Param questionnaire_version query string false "问卷版本"
// @Param page query int false "页码"
// @Param page_size query int false "每页数量"
// @Success 200 {object} core.Response{data=response.PublishedAssessmentModelListResponse}
// @Router /api/v1/assessment-models/published [get]
func (h *AssessmentModelHandler) ListPublished(c *gin.Context) {
	input, err := modelListInput(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	actor, err := assessmentModelActorContext(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	result, err := h.query.ListPublished(c.Request.Context(), actor, input)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, (*response.PublishedAssessmentModelListResponse)(result))
}

// @Summary 获取热门已发布测评模型
// @Tags AssessmentModel
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param kind query string false "模型类型"
// @Param limit query int false "数量"
// @Param window_days query int false "统计窗口天数"
// @Success 200 {object} core.Response{data=response.HotAssessmentModelListResponse}
// @Router /api/v1/assessment-models/hot [get]
func (h *AssessmentModelHandler) ListHot(c *gin.Context) {
	input, err := modelListInput(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	limit, err := queryPositiveInt(c, "limit", 5)
	if err != nil {
		h.Error(c, err)
		return
	}
	windowDays, err := queryPositiveInt(c, "window_days", 30)
	if err != nil {
		h.Error(c, err)
		return
	}
	actor, err := assessmentModelActorContext(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	result, err := h.query.ListHotPublished(c.Request.Context(), actor, input, limit, windowDays)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, (*response.HotAssessmentModelListResponse)(result))
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

func (h *AssessmentModelHandler) modelCode(c *gin.Context) string { return c.Param("code") }

func modelListInput(c *gin.Context) (modelcatalog.ListModelsDTO, error) {
	page, err := queryPositiveInt(c, "page", 1)
	if err != nil {
		return modelcatalog.ListModelsDTO{}, err
	}
	pageSize, err := queryPositiveInt(c, "page_size", 20)
	if err != nil {
		return modelcatalog.ListModelsDTO{}, err
	}
	return modelcatalog.ListModelsDTO{Kind: c.Query("kind"), SubKind: c.Query("sub_kind"), Status: c.Query("status"), Keyword: c.Query("keyword"), Category: c.Query("category"), Algorithm: c.Query("algorithm"), ProductChannel: c.Query("product_channel"), QuestionnaireCode: c.Query("questionnaire_code"), QuestionnaireVersion: c.Query("questionnaire_version"), Page: page, PageSize: pageSize}, nil
}

func queryPositiveInt(c *gin.Context, key string, fallback int) (int, error) {
	value, err := strconv.Atoi(c.DefaultQuery(key, strconv.Itoa(fallback)))
	if err != nil || value <= 0 {
		return 0, errors.WithCode(code.ErrInvalidArgument, "%s is invalid", key)
	}
	return value, nil
}
