package handler

import (
	"github.com/FangcunMount/qs-server/internal/collection-server/application/scale"
	"github.com/gin-gonic/gin"
)

// ScaleHandler 量表处理器
type ScaleHandler struct {
	*BaseHandler
	queryService *scale.QueryService
}

// NewScaleHandler 创建量表处理器
func NewScaleHandler(queryService *scale.QueryService) *ScaleHandler {
	return &ScaleHandler{
		BaseHandler:  NewBaseHandler(),
		queryService: queryService,
	}
}

// Get 获取量表详情
// @Summary 获取量表详情
// @Description 根据量表编码获取量表详情
// @Tags 量表
// @Produce json
// @Param code path string true "量表编码"
// @Success 200 {object} core.Response{data=scale.ScaleResponse}
// @Failure 400 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /api/v1/scales/{code} [get]
func (h *ScaleHandler) Get(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		h.BadRequestResponse(c, "code is required", nil)
		return
	}

	result, err := h.queryService.Get(c.Request.Context(), code)
	if err != nil {
		h.InternalErrorResponse(c, "get scale failed", err)
		return
	}

	if result == nil {
		h.NotFoundResponse(c, "scale not found", nil)
		return
	}

	h.Success(c, result)
}

// List 获取量表列表
// @Summary 获取量表列表
// @Description 分页获取量表列表，支持按主类、阶段、使用年龄、填报人、标签等条件过滤
// @Tags 量表
// @Produce json
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Param status query string false "状态过滤"
// @Param title query string false "标题过滤"
// @Param category query string false "主类过滤"
// @Param stage query string false "阶段过滤"
// @Param applicable_age query string false "使用年龄过滤"
// @Param reporter query string false "填报人过滤"
// @Param tags query []string false "标签过滤"
// @Success 200 {object} core.Response{data=scale.ListScalesResponse}
// @Failure 400 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /api/v1/scales [get]
func (h *ScaleHandler) List(c *gin.Context) {
	var req scale.ListScalesRequest
	if err := h.BindQuery(c, &req); err != nil {
		return
	}

	result, err := h.queryService.List(c.Request.Context(), &req)
	if err != nil {
		h.InternalErrorResponse(c, "list scales failed", err)
		return
	}

	h.Success(c, result)
}

// GetCategories 获取量表分类列表
// @Summary 获取量表分类列表
// @Description 获取量表的主类、阶段、使用年龄、填报人和标签等分类选项列表
// @Tags 量表
// @Produce json
// @Success 200 {object} core.Response{data=scale.ScaleCategoriesResponse}
// @Failure 500 {object} core.ErrResponse
// @Router /api/v1/scales/categories [get]
func (h *ScaleHandler) GetCategories(c *gin.Context) {
	result, err := h.queryService.GetCategories(c.Request.Context())
	if err != nil {
		h.InternalErrorResponse(c, "get scale categories failed", err)
		return
	}

	h.Success(c, result)
}

