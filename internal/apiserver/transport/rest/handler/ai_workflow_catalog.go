package handler

import (
	"errors"
	"strconv"

	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type AIWorkflowCatalogHandler struct {
	*BaseHandler
	service *app.AssetCatalogAdministration
}

func NewAIWorkflowCatalogHandler(s *app.AssetCatalogAdministration) *AIWorkflowCatalogHandler {
	return &AIWorkflowCatalogHandler{NewBaseHandler(), s}
}
func (h *AIWorkflowCatalogHandler) scope(c *gin.Context) (app.DraftScope, bool) {
	org, user, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return app.DraftScope{}, false
	}
	return app.DraftScope{OrganizationID: org, OperatorUserID: user}, true
}
func (h *AIWorkflowCatalogHandler) failure(c *gin.Context, err error) {
	errorCode, message := code.ErrUnknown, "AI asset catalog unavailable"
	switch {
	case errors.Is(err, app.ErrInvalid), status.Code(err) == codes.InvalidArgument:
		errorCode, message = code.ErrInvalidArgument, "Invalid AI asset query"
	case errors.Is(err, app.ErrGovernanceDenied), status.Code(err) == codes.PermissionDenied:
		errorCode, message = code.ErrPermissionDenied, "AI asset audit permission required"
	case errors.Is(err, app.ErrNotFound), status.Code(err) == codes.NotFound:
		errorCode, message = code.ErrPageNotFound, "AI asset unavailable"
	case errors.Is(err, app.ErrConflict), status.Code(err) == codes.Aborted:
		errorCode, message = code.ErrConflict, "AI asset response differs from confirmed query"
	case errors.Is(err, app.ErrManagementUnavailable):
		errorCode, message = code.ErrUnsupportedOperation, "AI asset catalog disabled"
	}
	h.Error(c, cberrors.WithCode(errorCode, "%s", message))
}

// List godoc
// @Summary 查询 qs-ai 共享不可变资产目录
// @Description 需要当前机构解读审计权限。按种类及精确 identity 分页，仅返回引用，不代表批准或生效。游标绑定筛选，跨页不保证目录快照。
// @Tags AI-Workflow-Assets
// @Produce json
// @Param kind path string true "profile/prompt/route/schema/suite"
// @Param identity query string false "精确资产标识"
// @Param limit query int false "每页 1–50，默认 20"
// @Param cursor query string false "上一页游标"
// @Success 200 {object} core.Response{data=app.AssetCatalogPage}
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 403 {object} core.ErrResponse
// @Failure 409 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /internal/v2/interpretation/ai-workflow/assets/{kind} [get]
func (h *AIWorkflowCatalogHandler) List(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	if len(c.Request.URL.RawQuery) > 16384 {
		h.failure(c, app.ErrInvalid)
		return
	}
	limit := 20
	if value, exists := c.GetQuery("limit"); exists {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 50 {
			h.failure(c, app.ErrInvalid)
			return
		}
		limit = parsed
	}
	page, err := h.service.List(c.Request.Context(), scope, app.AssetCatalogQuery{Kind: c.Param("kind"), Identity: c.Query("identity"), Limit: limit, Cursor: c.Query("cursor")})
	if err != nil {
		h.failure(c, err)
		return
	}
	h.Success(c, page)
}

// Get godoc
// @Summary 查询 qs-ai 不可变资产原文
// @Description 需要当前机构解读审计权限。使用查询参数承载含斜线的标识及版本；核对原始字节摘要，不返回命令审计或发布状态。
// @Tags AI-Workflow-Assets
// @Produce json
// @Param kind path string true "profile/prompt/route/schema/suite"
// @Param identity query string true "精确资产标识"
// @Param version query string true "精确版本"
// @Success 200 {object} core.Response{data=app.AssetCatalogDetail}
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 403 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 409 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /internal/v2/interpretation/ai-workflow/assets/{kind}/detail [get]
func (h *AIWorkflowCatalogHandler) Get(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	if len(c.Request.URL.RawQuery) > 16384 {
		h.failure(c, app.ErrInvalid)
		return
	}
	value, err := h.service.Get(c.Request.Context(), scope, app.AssetCatalogGet{Kind: c.Param("kind"), Identity: c.Query("identity"), Version: c.Query("version")})
	if err != nil {
		h.failure(c, err)
		return
	}
	h.Success(c, value)
}
