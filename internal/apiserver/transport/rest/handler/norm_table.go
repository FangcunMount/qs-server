package handler

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	modelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/request"
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/response"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/gin-gonic/gin"
)

type NormTableHandler struct {
	BaseHandler
	service modelcatalog.NormTableService
}

func NewNormTableHandler(service modelcatalog.NormTableService) *NormTableHandler {
	return &NormTableHandler{service: service}
}

// Import adds immutable norm reference material. Repeating byte-equivalent
// domain content is idempotent; reusing a version for different content is a conflict.
// @Summary 导入版本化常模表
// @Tags NormTable
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param request body request.ImportNormTableRequest true "常模表"
// @Success 200 {object} core.Response{data=response.NormTableDetailResponse}
// @Failure 400 {object} core.Response
// @Failure 409 {object} core.Response
// @Router /api/v1/norm-tables [post]
func (h *NormTableHandler) Import(c *gin.Context) {
	var req request.ImportNormTableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Error(c, errors.WithCode(code.ErrBind, "invalid norm table request: %v", err))
		return
	}
	actor, err := assessmentModelActorContext(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	result, err := h.service.Import(c.Request.Context(), actor, req.ToDomain())
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, (*response.NormTableDetailResponse)(result))
}

// List returns immutable norm-table summaries.
// @Summary 获取常模表列表
// @Tags NormTable
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param kind query string false "模型类型"
// @Param algorithm query string false "算法"
// @Param form_variant query string false "表单变体"
// @Param page query int false "页码"
// @Param page_size query int false "每页数量"
// @Success 200 {object} core.Response{data=response.NormTableListResponse}
// @Router /api/v1/norm-tables [get]
func (h *NormTableHandler) List(c *gin.Context) {
	page, err := queryPositiveInt(c, "page", 1)
	if err != nil {
		h.Error(c, err)
		return
	}
	pageSize, err := queryPositiveInt(c, "page_size", 20)
	if err != nil {
		h.Error(c, err)
		return
	}
	actor, err := assessmentModelActorContext(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	result, err := h.service.List(c.Request.Context(), actor, modelcatalog.ListNormTablesDTO{Kind: c.Query("kind"), Algorithm: c.Query("algorithm"), FormVariant: c.Query("form_variant"), Page: page, PageSize: pageSize})
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, (*response.NormTableListResponse)(result))
}

// Get returns one immutable norm table by version.
// @Summary 获取常模表详情
// @Tags NormTable
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param version path string true "常模表版本"
// @Success 200 {object} core.Response{data=response.NormTableDetailResponse}
// @Failure 404 {object} core.Response
// @Router /api/v1/norm-tables/{version} [get]
func (h *NormTableHandler) Get(c *gin.Context) {
	actor, err := assessmentModelActorContext(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	result, err := h.service.Get(c.Request.Context(), actor, c.Param("version"))
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, (*response.NormTableDetailResponse)(result))
}
