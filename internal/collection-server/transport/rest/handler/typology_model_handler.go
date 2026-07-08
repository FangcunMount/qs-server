package handler

import (
	"github.com/FangcunMount/qs-server/internal/collection-server/application/typologymodel"
	"github.com/gin-gonic/gin"
)

type TypologyModelHandler struct {
	*BaseHandler
	queryService *typologymodel.QueryService
}

func NewTypologyModelHandler(queryService *typologymodel.QueryService) *TypologyModelHandler {
	return &TypologyModelHandler{
		BaseHandler:  NewBaseHandler(),
		queryService: queryService,
	}
}

// Get returns a published typology model detail.
// @Summary 获取类型学模型详情
// @Tags 类型学模型
// @Produce json
// @Param code path string true "模型编码"
// @Success 200 {object} core.Response{data=typologymodel.TypologyModelResponse}
// @Router /api/v1/typology-models/{code} [get]
func (h *TypologyModelHandler) Get(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		h.BadRequestResponse(c, "code is required", nil)
		return
	}
	result, err := h.queryService.Get(c.Request.Context(), code)
	if err != nil {
		h.InternalErrorResponse(c, "get typology model failed", err)
		return
	}
	if result == nil {
		h.NotFoundResponse(c, "typology model not found", nil)
		return
	}
	h.Success(c, result)
}

// List returns published typology model summaries.
// @Summary 获取类型学模型列表
// @Description 浏览已发布类型学模型目录。model.kind/product_channel canonical 为 typology（R128b）。单模型详情与题版绑定请用 GET /typology-models/{code} 或推荐入口 POST /typology-assessment-sessions。
// @Tags 类型学模型
// @Produce json
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} core.Response{data=typologymodel.ListTypologyModelsResponse}
// @Router /api/v1/typology-models [get]
func (h *TypologyModelHandler) List(c *gin.Context) {
	var req typologymodel.ListTypologyModelsRequest
	if err := h.BindQuery(c, &req); err != nil {
		return
	}
	result, err := h.queryService.List(c.Request.Context(), &req)
	if err != nil {
		h.InternalErrorResponse(c, "list typology models failed", err)
		return
	}
	h.Success(c, result)
}

// GetCategories returns typology model algorithm categories.
// @Summary 获取类型学模型分类
// @Tags 类型学模型
// @Produce json
// @Success 200 {object} core.Response{data=typologymodel.TypologyModelCategoriesResponse}
// @Router /api/v1/typology-models/categories [get]
func (h *TypologyModelHandler) GetCategories(c *gin.Context) {
	result, err := h.queryService.GetCategories(c.Request.Context())
	if err != nil {
		h.InternalErrorResponse(c, "get typology model categories failed", err)
		return
	}
	h.Success(c, result)
}
