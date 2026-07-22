package handler

import (
	"strconv"
	"strings"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	authzapp "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	statisticsDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/gin-gonic/gin"
)

type StatisticsHandler struct {
	*BaseHandler
	read        *statisticsApp.ReadService
	coordinator *statisticsApp.Coordinator
	runs        statisticsApp.RunStore
}

func NewStatisticsHandler(read *statisticsApp.ReadService, coordinator *statisticsApp.Coordinator, runs statisticsApp.RunStore) *StatisticsHandler {
	return &StatisticsHandler{BaseHandler: NewBaseHandler(), read: read, coordinator: coordinator, runs: runs}
}

func statisticsFilter(c *gin.Context) statisticsApp.QueryFilter {
	return statisticsApp.QueryFilter{Preset: c.Query("preset"), From: c.Query("from"), To: c.Query("to")}
}

func parseStatisticsPage(c *gin.Context) (int, int, error) {
	page, size := 1, 20
	var err error
	if raw := c.Query("page"); raw != "" {
		page, err = strconv.Atoi(raw)
		if err != nil {
			return 0, 0, errors.WithCode(code.ErrInvalidArgument, "invalid page")
		}
	}
	if raw := c.Query("page_size"); raw != "" {
		size, err = strconv.Atoi(raw)
		if err != nil {
			return 0, 0, errors.WithCode(code.ErrInvalidArgument, "invalid page_size")
		}
	}
	return page, size, nil
}

// Overview godoc
// @Summary 查询 Statistics 机构总览
// @Tags Statistics
// @Param preset query string false "latest_complete_day/7d/30d/custom"
// @Param from query string false "上海日期 YYYY-MM-DD"
// @Param to query string false "上海日期 YYYY-MM-DD"
// @Success 200 {object} core.Response{data=statisticsApp.Overview}
// @Failure 503 {object} core.ErrResponse
// @Router /api/v2/statistics/overview [get]
func (h *StatisticsHandler) Overview(c *gin.Context) {
	orgID, err := h.RequireProtectedOrgID(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	value, err := h.read.Overview(c.Request.Context(), orgID, statisticsFilter(c))
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, value)
}

// Clinicians godoc
// @Summary 查询 Statistics 医生列表
// @Tags Statistics
// @Success 200 {object} core.Response{data=statisticsApp.Page[statisticsApp.ClinicianItem]}
// @Failure 503 {object} core.ErrResponse
// @Router /api/v2/statistics/clinicians [get]
func (h *StatisticsHandler) Clinicians(c *gin.Context) {
	orgID, err := h.RequireProtectedOrgID(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	page, size, err := parseStatisticsPage(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	value, err := h.read.Clinicians(c.Request.Context(), orgID, nil, nil, statisticsFilter(c), page, size)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, value)
}

// Clinician godoc
// @Summary 查询 Statistics 医生详情
// @Tags Statistics
// @Param id path uint64 true "医生 ID"
// @Success 200 {object} core.Response{data=StatisticsClinicianDetailResponse}
// @Failure 503 {object} core.ErrResponse
// @Router /api/v2/statistics/clinicians/{id} [get]
func (h *StatisticsHandler) Clinician(c *gin.Context) {
	orgID, err := h.RequireProtectedOrgID(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "invalid clinician id"))
		return
	}
	value, err := h.read.Clinicians(c.Request.Context(), orgID, &id, nil, statisticsFilter(c), 1, 1)
	if err != nil {
		h.Error(c, err)
		return
	}
	if len(value.Items) == 0 {
		h.Error(c, errors.WithCode(code.ErrPageNotFound, "clinician not found"))
		return
	}
	h.Success(c, StatisticsClinicianDetailResponse{Item: value.Items[0], TimeRange: value.TimeRange, Freshness: value.Freshness})
}

// CurrentClinicianOverview godoc
// @Summary 查询当前医生 Statistics 总览
// @Tags Statistics
// @Success 200 {object} core.Response{data=StatisticsClinicianDetailResponse}
// @Failure 503 {object} core.ErrResponse
// @Router /api/v2/statistics/clinicians/me/overview [get]
func (h *StatisticsHandler) CurrentClinicianOverview(c *gin.Context) {
	orgID, userID, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	value, err := h.read.Clinicians(c.Request.Context(), orgID, nil, &userID, statisticsFilter(c), 1, 1)
	if err != nil {
		h.Error(c, err)
		return
	}
	if len(value.Items) == 0 {
		h.Error(c, errors.WithCode(code.ErrPermissionDenied, "current operator is not an active clinician"))
		return
	}
	h.Success(c, StatisticsClinicianDetailResponse{Item: value.Items[0], TimeRange: value.TimeRange, Freshness: value.Freshness})
}

// Entries godoc
// @Summary 查询 Statistics 入口列表
// @Tags Statistics
// @Success 200 {object} core.Response{data=statisticsApp.Page[statisticsApp.EntryItem]}
// @Failure 503 {object} core.ErrResponse
// @Router /api/v2/statistics/entries [get]
func (h *StatisticsHandler) Entries(c *gin.Context) {
	orgID, err := h.RequireProtectedOrgID(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	page, size, err := parseStatisticsPage(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	var clinicianID *uint64
	if raw := c.Query("clinician_id"); raw != "" {
		id, parseErr := strconv.ParseUint(raw, 10, 64)
		if parseErr != nil {
			h.Error(c, errors.WithCode(code.ErrInvalidArgument, "invalid clinician_id"))
			return
		}
		clinicianID = &id
	}
	var active *bool
	if raw := c.Query("status"); raw != "" {
		if raw != "active" && raw != "inactive" {
			h.Error(c, errors.WithCode(code.ErrInvalidArgument, "invalid entry status"))
			return
		}
		value := raw == "active"
		active = &value
	}
	value, err := h.read.Entries(c.Request.Context(), orgID, nil, clinicianID, active, statisticsFilter(c), page, size)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, value)
}

// Entry godoc
// @Summary 查询 Statistics 入口详情
// @Tags Statistics
// @Param id path uint64 true "入口 ID"
// @Success 200 {object} core.Response{data=StatisticsEntryDetailResponse}
// @Failure 503 {object} core.ErrResponse
// @Router /api/v2/statistics/entries/{id} [get]
func (h *StatisticsHandler) Entry(c *gin.Context) {
	orgID, err := h.RequireProtectedOrgID(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "invalid entry id"))
		return
	}
	// The entry read port filters by clinician, so retrieve one page and select
	// the organization-scoped identifier explicitly.
	value, err := h.read.Entries(c.Request.Context(), orgID, &id, nil, nil, statisticsFilter(c), 1, 1)
	if err != nil {
		h.Error(c, err)
		return
	}
	if len(value.Items) > 0 {
		h.Success(c, StatisticsEntryDetailResponse{Item: value.Items[0], TimeRange: value.TimeRange, Freshness: value.Freshness})
		return
	}
	h.Error(c, errors.WithCode(code.ErrPageNotFound, "entry not found"))
}

// CurrentClinicianEntries godoc
// @Summary 查询当前医生 Statistics 入口
// @Tags Statistics
// @Success 200 {object} core.Response{data=statisticsApp.Page[statisticsApp.EntryItem]}
// @Failure 503 {object} core.ErrResponse
// @Router /api/v2/statistics/clinicians/me/entries [get]
func (h *StatisticsHandler) CurrentClinicianEntries(c *gin.Context) {
	orgID, userID, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	clinicianID, err := h.read.CurrentClinicianID(c.Request.Context(), orgID, userID)
	if err != nil {
		h.Error(c, err)
		return
	}
	page, size, err := parseStatisticsPage(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	value, err := h.read.Entries(c.Request.Context(), orgID, nil, &clinicianID, nil, statisticsFilter(c), page, size)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, value)
}

// CurrentClinicianTestees godoc
// @Summary 查询当前医生 Statistics 受试者摘要
// @Tags Statistics
// @Success 200 {object} core.Response{data=statisticsApp.TesteeSummary}
// @Failure 503 {object} core.ErrResponse
// @Router /api/v2/statistics/clinicians/me/testees-summary [get]
func (h *StatisticsHandler) CurrentClinicianTestees(c *gin.Context) {
	orgID, userID, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	value, err := h.read.CurrentClinicianTesteeSummary(c.Request.Context(), orgID, userID, statisticsFilter(c))
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, value)
}

type StatisticsContentRequest struct {
	Items []statisticsApp.ContentRef `json:"items"`
}

type StatisticsClinicianDetailResponse struct {
	Item      statisticsApp.ClinicianItem `json:"item"`
	TimeRange statisticsApp.DateRange     `json:"time_range"`
	Freshness statisticsApp.Freshness     `json:"freshness"`
}

type StatisticsEntryDetailResponse struct {
	Item      statisticsApp.EntryItem `json:"item"`
	TimeRange statisticsApp.DateRange `json:"time_range"`
	Freshness statisticsApp.Freshness `json:"freshness"`
}

type StatisticsRunListResponse struct {
	Items []statisticsApp.Run `json:"items"`
}

type StatisticsResumeCacheRequest struct {
	Reason  string `json:"reason"`
	Confirm bool   `json:"confirm"`
}

// Contents godoc
// @Summary 批量查询 Statistics 内容统计
// @Tags Statistics
// @Param request body StatisticsContentRequest true "内容引用"
// @Success 200 {object} core.Response{data=statisticsApp.ContentBatch}
// @Failure 503 {object} core.ErrResponse
// @Router /api/v2/statistics/contents/batch [post]
func (h *StatisticsHandler) Contents(c *gin.Context) {
	var request StatisticsContentRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "invalid request body"))
		return
	}
	if len(request.Items) == 0 || len(request.Items) > 100 {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "items must contain 1..100 entries"))
		return
	}
	allowed := map[string]bool{"questionnaire": true, "scale": true, "typology": true, "behavioral_rating": true, "cognitive": true}
	seen := map[string]bool{}
	snapshot, _ := authzapp.FromContext(c.Request.Context())
	canQuestionnaire := authzapp.DecideCapability(snapshot, authzapp.CapabilityManageQuestionnaires).Allowed
	canModel := authzapp.DecideCapability(snapshot, authzapp.CapabilityManageAssessmentModels).Allowed
	for index := range request.Items {
		request.Items[index].Kind = strings.TrimSpace(request.Items[index].Kind)
		request.Items[index].Code = strings.TrimSpace(request.Items[index].Code)
		item := request.Items[index]
		key := item.Kind + "\x00" + item.Code
		if !allowed[item.Kind] || item.Code == "" || seen[key] {
			h.Error(c, errors.WithCode(code.ErrInvalidArgument, "invalid or duplicate content item"))
			return
		}
		if (item.Kind == "questionnaire" && !canQuestionnaire) || (item.Kind != "questionnaire" && !canModel) {
			h.Error(c, errors.WithCode(code.ErrPermissionDenied, "content kind is outside caller capability"))
			return
		}
		seen[key] = true
	}
	orgID, err := h.RequireProtectedOrgID(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	value, err := h.read.Contents(c.Request.Context(), orgID, request.Items)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, value)
}

type StatisticsRunRequest struct {
	Mode         string `json:"mode"`
	FromDate     string `json:"from_date"`
	ToDate       string `json:"to_date"`
	Reason       string `json:"reason"`
	Confirm      bool   `json:"confirm"`
	ValidateOnly bool   `json:"validate_only"`
}

// CreateRun godoc
// @Summary 创建 Statistics 同步批次
// @Tags Statistics-Internal
// @Param request body StatisticsRunRequest true "批次窗口"
// @Success 200 {object} core.Response{data=statisticsApp.Run}
// @Router /internal/v2/statistics/runs [post]
func (h *StatisticsHandler) CreateRun(c *gin.Context) {
	orgID, userID, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	var request StatisticsRunRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "invalid request body"))
		return
	}
	mode := statisticsDomain.RunMode(strings.TrimSpace(request.Mode))
	isValidate := request.ValidateOnly || mode == statisticsDomain.RunModeValidate
	if !isValidate && !request.Confirm {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "confirm=true is required"))
		return
	}
	if strings.TrimSpace(request.Reason) == "" {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "reason is required"))
		return
	}
	if len([]rune(request.Reason)) > 500 {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "reason exceeds 500 characters"))
		return
	}
	from, err := time.ParseInLocation("2006-01-02", request.FromDate, statisticsDomain.Shanghai)
	if err != nil {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "invalid from_date"))
		return
	}
	to, err := time.ParseInLocation("2006-01-02", request.ToDate, statisticsDomain.Shanghai)
	if err != nil {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "invalid to_date"))
		return
	}
	run, runErr := h.coordinator.Run(c.Request.Context(), statisticsApp.RunRequest{OrgID: orgID, FromDate: from, ToDate: to, Reason: strings.TrimSpace(request.Reason), TriggerType: "manual", OperatorID: uint64(userID), Mode: mode, ValidateOnly: request.ValidateOnly})
	if statisticsApp.IsInvalidRunRequest(runErr) {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "%s", runErr.Error()))
		return
	}
	if runErr != nil && (run == nil || run.Status != statisticsDomain.RunStatusDataCommitted) {
		h.Error(c, runErr)
		return
	}
	h.Success(c, run)
}

// ListRuns godoc
// @Summary 查询 Statistics 同步批次
// @Tags Statistics-Internal
// @Success 200 {object} core.Response{data=StatisticsRunListResponse}
// @Router /internal/v2/statistics/runs [get]
func (h *StatisticsHandler) ListRuns(c *gin.Context) {
	orgID, err := h.RequireProtectedOrgID(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	limit := 20
	if raw := c.Query("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil {
			h.Error(c, errors.WithCode(code.ErrInvalidArgument, "invalid limit"))
			return
		}
	}
	runs, err := h.runs.List(c.Request.Context(), orgID, limit)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, StatisticsRunListResponse{Items: runs})
}

// GetRun godoc
// @Summary 查询 Statistics 同步批次详情
// @Tags Statistics-Internal
// @Param id path uint64 true "批次 ID"
// @Success 200 {object} core.Response{data=statisticsApp.Run}
// @Router /internal/v2/statistics/runs/{id} [get]
func (h *StatisticsHandler) GetRun(c *gin.Context) {
	orgID, err := h.RequireProtectedOrgID(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "invalid run id"))
		return
	}
	run, err := h.runs.Get(c.Request.Context(), id)
	if err != nil {
		h.Error(c, err)
		return
	}
	if run == nil || run.OrgID != orgID {
		h.Error(c, errors.WithCode(code.ErrPageNotFound, "statistics run not found"))
		return
	}
	h.Success(c, run)
}

// ResumeCache godoc
// @Summary 恢复 Statistics 批次缓存发布
// @Tags Statistics-Internal
// @Param id path uint64 true "批次 ID"
// @Param request body StatisticsResumeCacheRequest true "审计原因与明确确认"
// @Success 200 {object} core.Response{data=statisticsApp.Run}
// @Router /internal/v2/statistics/runs/{id}/resume-cache [post]
func (h *StatisticsHandler) ResumeCache(c *gin.Context) {
	orgID, userID, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "invalid run id"))
		return
	}
	existing, err := h.runs.Get(c.Request.Context(), id)
	if err != nil {
		h.Error(c, err)
		return
	}
	if existing == nil || existing.OrgID != orgID {
		h.Error(c, errors.WithCode(code.ErrPageNotFound, "statistics run not found"))
		return
	}
	var request StatisticsResumeCacheRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "invalid request body"))
		return
	}
	if !request.Confirm {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "confirm=true is required"))
		return
	}
	if strings.TrimSpace(request.Reason) == "" {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "reason is required"))
		return
	}
	if len([]rune(request.Reason)) > 500 {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "reason exceeds 500 characters"))
		return
	}
	run, err := h.coordinator.ResumeCache(c.Request.Context(), id, statisticsApp.CacheResumeRequest{OperatorID: uint64(userID), Reason: strings.TrimSpace(request.Reason)})
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, run)
}
