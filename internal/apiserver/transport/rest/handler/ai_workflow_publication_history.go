package handler

import (
	"net/url"
	"strconv"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/gin-gonic/gin"
)

func historySelector(values url.Values) (app.PublicationSelector, bool) {
	for _, key := range []string{"audience", "model_kind", "decision_kind", "model_code", "model_version", "limit", "before_version"} {
		if v, exists := values[key]; exists && len(v) != 1 {
			return app.PublicationSelector{}, false
		}
	}
	value := app.PublicationSelector{Audience: values.Get("audience"), ModelKind: values.Get("model_kind"), DecisionKind: values.Get("decision_kind")}
	if v, ok := values["model_code"]; ok {
		value.ModelCode = &v[0]
	}
	if v, ok := values["model_version"]; ok {
		value.ModelVersion = &v[0]
	}
	return value, value.Valid()
}

// ListHistory godoc
// @Summary 查询 qs-ai 配置发布历史
// @Description 需要当前解读审计权限；精确 selector、降序排他版本游标，保留原操作者。只读查询不重放命令，默认 limit=20、before_version=0。管理开关默认关闭。
// @Tags AI-Workflow-Publications
// @Produce json
// @Param audience query string true "participant"
// @Param model_kind query string true "scale"
// @Param decision_kind query string true "score_range"
// @Param model_code query string false "测评编码"
// @Param model_version query string false "测评版本，需要编码"
// @Param limit query int false "每页 1–20 条，默认 20"
// @Param before_version query int false "排他版本游标，0 从最新开始"
// @Success 200 {object} core.Response{data=app.PublicationHistoryPage}
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 403 {object} core.ErrResponse
// @Failure 409 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /internal/v2/interpretation/ai-workflow/publications/history [get]
func (h *AIWorkflowPublicationHandler) ListHistory(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	values := c.Request.URL.Query()
	selector, ok := historySelector(values)
	query := app.PublicationHistoryQuery{Selector: selector, Limit: 20}
	if !ok {
		h.failure(c, app.ErrInvalid)
		return
	}
	if value, exists := values["before_version"]; exists {
		parsed, err := strconv.ParseInt(value[0], 10, 64)
		if err != nil {
			h.failure(c, app.ErrInvalid)
			return
		}
		query.BeforeVersion = parsed
	}
	if value, exists := values["limit"]; exists {
		parsed, err := strconv.ParseInt(value[0], 10, 32)
		if err != nil {
			h.failure(c, app.ErrInvalid)
			return
		}
		query.Limit = int32(parsed)
	}
	result, err := h.service.ListHistory(c.Request.Context(), scope, query)
	if err != nil {
		h.failure(c, err)
		return
	}
	h.Success(c, result)
}

// GetHistory godoc
// @Summary 核对 qs-ai 历史发布版本的原始证据
// @Description 需要当前解读审计权限；按精确 selector 和版本读取原前后发布及审计，actor 保留原操作者。与原命令 GetReceipt 的操作者限制区分，不改变发布状态或授予回退权限。
// @Tags AI-Workflow-Publications
// @Produce json
// @Param version path int true "正历史版本"
// @Param audience query string true "participant"
// @Param model_kind query string true "scale"
// @Param decision_kind query string true "score_range"
// @Param model_code query string false "测评编码"
// @Param model_version query string false "测评版本，需要编码"
// @Success 200 {object} core.Response{data=app.PublicationReceipt}
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 403 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 409 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /internal/v2/interpretation/ai-workflow/publications/history/{version} [get]
func (h *AIWorkflowPublicationHandler) GetHistory(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	values := c.Request.URL.Query()
	selector, ok := historySelector(values)
	version, err := strconv.ParseInt(c.Param("version"), 10, 64)
	if !ok || err != nil || values.Has("before_version") || values.Has("limit") {
		h.failure(c, app.ErrInvalid)
		return
	}
	result, err := h.service.GetHistory(c.Request.Context(), scope, selector, version)
	if err != nil {
		h.failure(c, err)
		return
	}
	h.Success(c, result)
}
