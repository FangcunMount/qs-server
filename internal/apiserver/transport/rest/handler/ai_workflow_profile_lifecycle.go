package handler

import (
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/gin-gonic/gin"
	"net/url"
	"strconv"
)

func profileLifecycleParams(c *gin.Context, allowed ...string) (url.Values, bool) {
	if len(c.Request.URL.RawQuery) > 8192 {
		return nil, false
	}
	values, err := url.ParseQuery(c.Request.URL.RawQuery)
	if err != nil {
		return nil, false
	}
	for key, v := range values {
		found := false
		for _, name := range allowed {
			if name == key {
				found = true
				break
			}
		}
		if !found || len(v) != 1 {
			return nil, false
		}
	}
	return values, true
}

// ListLifecycle godoc
// @Summary 查询 qs-ai Profile 发布状态和迁入来源
// @Description 需要解读审计权限；状态来自 qs-ai 发布记录。迁入不代表发布，旧 QS 调试版本不要求迁入。游标绑定筛选，跨页不保证同一快照。
// @Tags AI-Workflow-Profiles
// @Produce json
// @Param identity query string false "精确 Profile 标识"
// @Param status query string false "draft/published/disabled"
// @Param limit query int false "每页 1–50，默认 20"
// @Param cursor query string false "上一页游标"
// @Success 200 {object} core.Response{data=app.ProfileLifecyclePage}
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 403 {object} core.ErrResponse
// @Failure 409 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /internal/v2/interpretation/ai-workflow/profiles [get]
func (h *AIWorkflowProfileHandler) ListLifecycle(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	values, ok := profileLifecycleParams(c, "identity", "status", "limit", "cursor")
	if !ok {
		h.failure(c, app.ErrInvalid)
		return
	}
	limit := 20
	if values.Has("limit") {
		var err error
		limit, err = strconv.Atoi(values.Get("limit"))
		if err != nil || limit < 1 || limit > 50 {
			h.failure(c, app.ErrInvalid)
			return
		}
	}
	value, err := h.service.ListLifecycle(c.Request.Context(), scope, app.ProfileLifecycleQuery{Identity: values.Get("identity"), Status: values.Get("status"), Limit: limit, Cursor: values.Get("cursor")})
	if err != nil {
		h.failure(c, err)
		return
	}
	h.Success(c, value)
}

// GetLifecycle godoc
// @Summary 查询指定 qs-ai Profile 版本的当前发布状态
// @Description 需要解读审计权限。共享配置只返回来源、不可变引用和发布状态，不返回操作人审计。状态不授予发布权限。
// @Tags AI-Workflow-Profiles
// @Produce json
// @Param identity query string true "精确 Profile 标识"
// @Param version query string true "精确 Profile 版本"
// @Success 200 {object} core.Response{data=app.ProfileLifecycle}
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 403 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 409 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /internal/v2/interpretation/ai-workflow/profiles/lifecycle [get]
func (h *AIWorkflowProfileHandler) GetLifecycle(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	values, ok := profileLifecycleParams(c, "identity", "version")
	if !ok {
		h.failure(c, app.ErrInvalid)
		return
	}
	value, err := h.service.GetLifecycle(c.Request.Context(), scope, values.Get("identity"), values.Get("version"))
	if err != nil {
		h.failure(c, err)
		return
	}
	h.Success(c, value)
}
