package handler

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/gin-gonic/gin"
	"strconv"
	"strings"
)

func parseOperationsFilter(c *gin.Context) (app.OperationsFilter, error) {
	f := app.OperationsFilter{From: c.Query("from"), To: c.Query("to")}
	if raw, exists := c.GetQuery("store_ids"); exists {
		for _, v := range strings.Split(raw, ",") {
			id, err := strconv.ParseUint(v, 10, 64)
			if err != nil || id == 0 {
				return f, errors.WithCode(code.ErrInvalidArgument, "invalid store_ids")
			}
			f.StoreIDs = append(f.StoreIDs, id)
		}
	}
	return f, nil
}

// AnalysisOverview godoc
// @Summary 当前服务归属范围内的服务与计划统计
// @Tags Statistics
// @Param from query string false "上海日期，包含"
// @Param to query string false "上海日期，不包含"
// @Param store_ids query string false "门店字符串 ID，逗号分隔，必须为授权子集"
// @Success 200 {object} core.Response{data=app.AnalysisOverview}
// @Failure 403 {object} core.ErrResponse
// @Failure 503 {object} core.ErrResponse
// @Router /api/v2/statistics/operations/analysis/overview [get]
func (h *StatisticsHandler) AnalysisOverview(c *gin.Context) { h.analysis(c, "overview") }

// AnalysisClinicians godoc
// @Summary 当前门店临床人员分页与全范围汇总
// @Tags Statistics
// @Param from query string false "上海日期，包含"
// @Param to query string false "上海日期，不包含"
// @Param store_ids query string false "授权门店 ID 集合"
// @Param page query int false "页码，默认 1"
// @Param page_size query int false "每页数量，默认 20，最多 100"
// @Success 200 {object} core.Response{data=app.AnalysisClinicianPage}
// @Failure 403 {object} core.ErrResponse
// @Failure 503 {object} core.ErrResponse
// @Router /api/v2/statistics/operations/analysis/clinicians [get]
func (h *StatisticsHandler) AnalysisClinicians(c *gin.Context) { h.analysis(c, "clinicians") }

// AnalysisEntries godoc
// @Summary 当前门店入口统计，不包含二维码凭证
// @Tags Statistics
// @Param from query string false "上海日期，包含"
// @Param to query string false "上海日期，不包含"
// @Param store_ids query string false "授权门店 ID 集合"
// @Param page query int false "页码，默认 1"
// @Param page_size query int false "每页数量，默认 20，最多 100"
// @Param clinician_id query string false "医生 ID"
// @Param is_active query boolean false "启用状态"
// @Success 200 {object} core.Response{data=app.AnalysisEntryPage}
// @Failure 403 {object} core.ErrResponse
// @Failure 503 {object} core.ErrResponse
// @Router /api/v2/statistics/operations/analysis/entries [get]
func (h *StatisticsHandler) AnalysisEntries(c *gin.Context) { h.analysis(c, "entries") }
func (h *StatisticsHandler) analysis(c *gin.Context, kind string) {
	org, err := h.RequireProtectedOrgID(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	f, err := parseOperationsFilter(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	if kind == "overview" {
		v, e := h.read.AnalysisOverview(c.Request.Context(), org, f)
		if e != nil {
			h.Error(c, e)
			return
		}
		h.Success(c, v)
		return
	}
	page, size, err := parseStatisticsPage(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	if page < 1 || size < 1 {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "invalid pagination"))
		return
	}
	if kind == "clinicians" {
		v, e := h.read.AnalysisClinicians(c.Request.Context(), org, f, page, size)
		if e != nil {
			h.Error(c, e)
			return
		}
		h.Success(c, v)
		return
	}
	var clinician *uint64
	var active *bool
	if raw, exists := c.GetQuery("clinician_id"); exists {
		id, e := strconv.ParseUint(raw, 10, 64)
		if e != nil || id == 0 {
			h.Error(c, errors.WithCode(code.ErrInvalidArgument, "invalid clinician_id"))
			return
		}
		clinician = &id
	}
	if raw, exists := c.GetQuery("is_active"); exists {
		v, e := strconv.ParseBool(raw)
		if e != nil {
			h.Error(c, errors.WithCode(code.ErrInvalidArgument, "invalid is_active"))
			return
		}
		active = &v
	}
	v, e := h.read.AnalysisEntries(c.Request.Context(), org, f, clinician, active, page, size)
	if e != nil {
		h.Error(c, e)
		return
	}
	h.Success(c, v)
}
