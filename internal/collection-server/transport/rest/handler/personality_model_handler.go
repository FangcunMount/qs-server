package handler

import (
	personalitymodel "github.com/FangcunMount/qs-server/internal/collection-server/application/personalitymodel"
	"github.com/gin-gonic/gin"
)

type PersonalityModelHandler struct {
	*BaseHandler
	queryService *personalitymodel.QueryService
}

func NewPersonalityModelHandler(queryService *personalitymodel.QueryService) *PersonalityModelHandler {
	return &PersonalityModelHandler{
		BaseHandler:  NewBaseHandler(),
		queryService: queryService,
	}
}

// Get returns a published personality model detail.
// @Summary 获取人格测评模型详情
// @Tags 人格测评模型
// @Produce json
// @Param code path string true "模型编码"
// @Success 200 {object} core.Response{data=personalitymodel.PersonalityModelResponse}
// @Router /api/v1/personality-models/{code} [get]
func (h *PersonalityModelHandler) Get(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		h.BadRequestResponse(c, "code is required", nil)
		return
	}
	result, err := h.queryService.Get(c.Request.Context(), code)
	if err != nil {
		h.InternalErrorResponse(c, "get personality model failed", err)
		return
	}
	if result == nil {
		h.NotFoundResponse(c, "personality model not found", nil)
		return
	}
	h.Success(c, result)
}

// List returns published personality model summaries.
// @Summary 获取人格测评模型列表
// @Tags 人格测评模型
// @Produce json
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Param algorithm query string false "算法过滤 mbti/sbti"
// @Success 200 {object} core.Response{data=personalitymodel.ListPersonalityModelsResponse}
// @Router /api/v1/personality-models [get]
func (h *PersonalityModelHandler) List(c *gin.Context) {
	var req personalitymodel.ListPersonalityModelsRequest
	if err := h.BindQuery(c, &req); err != nil {
		return
	}
	result, err := h.queryService.List(c.Request.Context(), &req)
	if err != nil {
		h.InternalErrorResponse(c, "list personality models failed", err)
		return
	}
	h.Success(c, result)
}

// GetCategories returns personality model algorithm categories.
// @Summary 获取人格测评模型分类
// @Tags 人格测评模型
// @Produce json
// @Success 200 {object} core.Response{data=personalitymodel.PersonalityModelCategoriesResponse}
// @Router /api/v1/personality-models/categories [get]
func (h *PersonalityModelHandler) GetCategories(c *gin.Context) {
	result, err := h.queryService.GetCategories(c.Request.Context())
	if err != nil {
		h.InternalErrorResponse(c, "get personality model categories failed", err)
		return
	}
	h.Success(c, result)
}
