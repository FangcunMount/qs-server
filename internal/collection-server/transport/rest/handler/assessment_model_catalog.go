package handler

import (
	"github.com/FangcunMount/qs-server/internal/collection-server/application/modelcatalog"
	"github.com/gin-gonic/gin"
)

// AssessmentModelCatalogHandler serves published model catalogue data for C
// clients. It never exposes draft configuration or a legacy scale DTO.
type AssessmentModelCatalogHandler struct {
	*BaseHandler
	query *modelcatalog.QueryService
}

func NewAssessmentModelCatalogHandler(query *modelcatalog.QueryService) *AssessmentModelCatalogHandler {
	return &AssessmentModelCatalogHandler{BaseHandler: NewBaseHandler(), query: query}
}

// Get returns one immutable published model, including canonical DefinitionV2.
// @Summary 获取已发布测评模型
// @Tags AssessmentModelCatalog
// @Produce json
// @Param code path string true "模型编码"
// @Success 200 {object} core.Response{data=modelcatalog.ModelResponse}
// @Router /api/v1/assessment-models/{code} [get]
func (h *AssessmentModelCatalogHandler) Get(c *gin.Context) {
	result, err := h.query.Get(c.Request.Context(), c.Param("code"))
	if err != nil {
		h.InternalErrorResponse(c, "get assessment model failed", err)
		return
	}
	if result == nil {
		h.NotFoundResponse(c, "published assessment model not found", nil)
		return
	}
	h.Success(c, result)
}

// List returns immutable published models.
// @Summary 获取已发布测评模型列表
// @Tags AssessmentModelCatalog
// @Produce json
// @Param kind query string false "模型类型"
// @Param page query int false "页码"
// @Param page_size query int false "每页数量"
// @Success 200 {object} core.Response{data=modelcatalog.ListResponse}
// @Router /api/v1/assessment-models [get]
func (h *AssessmentModelCatalogHandler) List(c *gin.Context) {
	var request modelcatalog.ListRequest
	if err := h.BindQuery(c, &request); err != nil {
		return
	}
	result, err := h.query.List(c.Request.Context(), &request)
	if err != nil {
		h.InternalErrorResponse(c, "list assessment models failed", err)
		return
	}
	h.Success(c, result)
}

// ListHot returns the scale hot-rank projection through the generic catalogue.
// @Summary 获取热门已发布测评模型
// @Tags AssessmentModelCatalog
// @Produce json
// @Success 200 {object} core.Response{data=modelcatalog.HotResponse}
// @Router /api/v1/assessment-models/hot [get]
func (h *AssessmentModelCatalogHandler) ListHot(c *gin.Context) {
	var request modelcatalog.HotRequest
	if err := h.BindQuery(c, &request); err != nil {
		return
	}
	result, err := h.query.ListHot(c.Request.Context(), &request)
	if err != nil {
		h.InternalErrorResponse(c, "list hot assessment models failed", err)
		return
	}
	h.Success(c, result)
}

// Options returns catalogue presentation options scoped by kind.
// @Summary 获取测评模型目录选项
// @Tags AssessmentModelCatalog
// @Produce json
// @Param kind query string false "模型类型"
// @Success 200 {object} core.Response{data=modelcatalog.OptionsResponse}
// @Router /api/v1/assessment-models/options [get]
func (h *AssessmentModelCatalogHandler) Options(c *gin.Context) {
	result, err := h.query.Options(c.Request.Context(), c.Query("kind"))
	if err != nil {
		h.InternalErrorResponse(c, "get assessment model options failed", err)
		return
	}
	h.Success(c, result)
}
