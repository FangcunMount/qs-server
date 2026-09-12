package handler

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/gin-gonic/gin"
	"strconv"
	"strings"
)

// OperationsOverview godoc
// @Summary 当前授权范围内运营统计
// @Tags Statistics
// @Param from query string false "上海日期，包含"
// @Param to query string false "上海日期，不包含"
// @Param store_ids query string false "门店字符串 ID，逗号分隔，必须为授权子集"
// @Success 200 {object} core.Response{data=app.OperationsOverview}
// @Failure 403 {object} core.ErrResponse
// @Failure 503 {object} core.ErrResponse
// @Router /api/v2/statistics/operations/overview [get]
func (h *StatisticsHandler) OperationsOverview(c *gin.Context) { h.operations(c) }

// OperationsStores godoc
// @Summary 授权门店指标与对比
// @Tags Statistics
// @Param from query string false "上海日期，包含"
// @Param to query string false "上海日期，不包含"
// @Param store_ids query string false "门店字符串 ID，逗号分隔"
// @Success 200 {object} core.Response{data=app.OperationsOverview}
// @Failure 403 {object} core.ErrResponse
// @Failure 503 {object} core.ErrResponse
// @Router /api/v2/statistics/operations/stores [get]
func (h *StatisticsHandler) OperationsStores(c *gin.Context) { h.operations(c) }
func (h *StatisticsHandler) operations(c *gin.Context) {
	org, err := h.RequireProtectedOrgID(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	filter := app.OperationsFilter{From: c.Query("from"), To: c.Query("to")}
	if raw := c.Query("store_ids"); raw != "" {
		for _, value := range strings.Split(raw, ",") {
			id, err := strconv.ParseUint(value, 10, 64)
			if err != nil || id == 0 {
				h.Error(c, errors.WithCode(code.ErrInvalidArgument, "invalid store_ids"))
				return
			}
			filter.StoreIDs = append(filter.StoreIDs, id)
		}
	}
	result, err := h.read.Operations(c.Request.Context(), org, filter)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, result)
}
