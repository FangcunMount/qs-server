package handler

import (
	"fmt"
	"strconv"
	"time"

	interpretationcatalog "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/catalogreconcile"
	interpretationclinician "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/clinician"
	interpretationoperations "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/operations"
	interpretationreporttemplate "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporttemplate"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/admission"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	domainreporttemplate "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/reporttemplate"
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/response"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/gin-gonic/gin"
)

type InterpretationReportTemplateHandler struct {
	*BaseHandler
	service interpretationreporttemplate.Service
}

func NewInterpretationReportTemplateHandler(service interpretationreporttemplate.Service) *InterpretationReportTemplateHandler {
	return &InterpretationReportTemplateHandler{BaseHandler: &BaseHandler{}, service: service}
}

type reportTemplateWire struct {
	TemplateID      string     `json:"template_id"`
	TemplateVersion string     `json:"template_version"`
	BuilderIdentity string     `json:"builder_identity"`
	AdapterKey      string     `json:"adapter_key,omitempty"`
	Status          string     `json:"status"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	PublishedAt     *time.Time `json:"published_at,omitempty"`
	PublishedBy     string     `json:"published_by,omitempty"`
	DisabledAt      *time.Time `json:"disabled_at,omitempty"`
	DisabledBy      string     `json:"disabled_by,omitempty"`
}

func reportTemplateResponse(item *domainreporttemplate.ReportTemplate) reportTemplateWire {
	return reportTemplateWire{
		TemplateID: item.TemplateID(), TemplateVersion: item.TemplateVersion().String(),
		BuilderIdentity: item.BuilderIdentity(), AdapterKey: item.AdapterKey(), Status: string(item.Status()),
		CreatedAt: item.CreatedAt(), UpdatedAt: item.UpdatedAt(), PublishedAt: item.PublishedAt(),
		PublishedBy: item.PublishedBy(), DisabledAt: item.DisabledAt(), DisabledBy: item.DisabledBy(),
	}
}

func (h *InterpretationReportTemplateHandler) List(c *gin.Context) {
	if _, _, err := h.RequireProtectedScope(c); err != nil {
		h.Error(c, err)
		return
	}
	templateID := c.Query("template_id")
	if templateID == "" {
		h.Error(c, fmt.Errorf("template_id is required"))
		return
	}
	items, err := h.service.List(c.Request.Context(), templateID, 100)
	if err != nil {
		h.Error(c, err)
		return
	}
	result := make([]reportTemplateWire, 0, len(items))
	for _, item := range items {
		result = append(result, reportTemplateResponse(item))
	}
	h.Success(c, result)
}

func (h *InterpretationReportTemplateHandler) Get(c *gin.Context) {
	if _, _, err := h.RequireProtectedScope(c); err != nil {
		h.Error(c, err)
		return
	}
	item, err := h.service.Get(c.Request.Context(), c.Param("template_id"), policy.TemplateVersion(c.Param("version")))
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, reportTemplateResponse(item))
}

func (h *InterpretationReportTemplateHandler) CreateDraft(c *gin.Context) {
	_, userID, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	var request struct {
		TemplateID      string `json:"template_id"`
		TemplateVersion string `json:"template_version"`
		BuilderIdentity string `json:"builder_identity"`
		AdapterKey      string `json:"adapter_key"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		h.Error(c, err)
		return
	}
	item, err := h.service.CreateDraft(c.Request.Context(), interpretationreporttemplate.CreateDraftCommand{
		Actor:      interpretationreporttemplate.Actor{OperatorUserID: userID},
		TemplateID: request.TemplateID, TemplateVersion: policy.TemplateVersion(request.TemplateVersion),
		BuilderIdentity: request.BuilderIdentity, AdapterKey: request.AdapterKey,
	})
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, reportTemplateResponse(item))
}

type InterpretationClinicianHandler struct {
	*BaseHandler
	service interpretationclinician.Service
}

type InterpretationCatalogReconcileHandler struct {
	*BaseHandler
	service interpretationcatalog.Service
}

func NewInterpretationCatalogReconcileHandler(s interpretationcatalog.Service) *InterpretationCatalogReconcileHandler {
	return &InterpretationCatalogReconcileHandler{BaseHandler: &BaseHandler{}, service: s}
}

// Reconcile godoc
// @Summary 只读检查当前组织的 Interpretation Catalog 漂移
// @Tags Interpretation-Operations
// @Produce json
// @Success 200 {object} core.Response{data=interpretationcatalog.DriftCounts}
// @Router /internal/v1/interpretation/catalog/reconcile [get]
func (h *InterpretationCatalogReconcileHandler) Reconcile(c *gin.Context) {
	orgID, _, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	result, err := h.service.ReconcileOnce(c.Request.Context(), interpretationcatalog.Filter{OrgID: &orgID})
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, result)
}

// ListDrifts godoc
// @Summary 分页查询当前组织的 Interpretation Catalog 漂移明细
// @Tags Interpretation-Operations
// @Produce json
// @Param kind query string true "missing|dangling|association_mismatch|wrong_winner"
// @Param cursor query string false "稳定游标"
// @Param limit query int false "批量" default(500)
// @Param assessment_id query string false "测评ID"
// @Router /internal/v1/interpretation/catalog/drifts [get]
func (h *InterpretationCatalogReconcileHandler) ListDrifts(c *gin.Context) {
	orgID, _, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	filter := interpretationcatalog.Filter{OrgID: &orgID, Kind: interpretationcatalog.DriftKind(c.Query("kind"))}
	if value := c.Query("assessment_id"); value != "" {
		assessmentID, err := strconv.ParseUint(value, 10, 64)
		if err != nil || assessmentID == 0 {
			h.Error(c, fmt.Errorf("invalid assessment_id"))
			return
		}
		filter.AssessmentID = &assessmentID
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "500"))
	page, err := h.service.ListDrifts(c.Request.Context(), filter, c.Query("cursor"), limit)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, page)
}

func (h *InterpretationCatalogReconcileHandler) CreateRepairPlan(c *gin.Context) {
	orgID, _, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	var request struct {
		AssessmentID uint64 `json:"assessment_id"`
		Kind         string `json:"kind"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		h.Error(c, err)
		return
	}
	plan, err := h.service.CreateRepairPlan(c.Request.Context(), orgID, interpretationcatalog.Filter{
		OrgID: &orgID, AssessmentID: &request.AssessmentID, Kind: interpretationcatalog.DriftKind(request.Kind),
	})
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, plan)
}

func NewInterpretationClinicianHandler(s interpretationclinician.Service) *InterpretationClinicianHandler {
	return &InterpretationClinicianHandler{BaseHandler: &BaseHandler{}, service: s}
}

// List godoc
// @Summary 查询当前临床人员获授权受试者报告
// @Tags Interpretation-Clinician
// @Produce json
// @Param testee_id path string true "受试者ID"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(10)
// @Success 200 {object} core.Response{data=response.ReportListResponse}
// @Router /api/v1/clinicians/me/testees/{testee_id}/reports [get]
func (h *InterpretationClinicianHandler) List(c *gin.Context) {
	org, user, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	testee, ok := parsePathUint(c, "testee_id", h.BaseHandler)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	result, err := h.service.ListParticipantReports(c.Request.Context(), interpretationclinician.Actor{OrgID: org, OperatorUserID: user}, interpretationclinician.ListQuery{TesteeID: testee, Page: page, PageSize: size})
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, response.NewReportListResponse(result))
}

// Get godoc
// @Summary 查询当前临床人员获授权受试者报告详情
// @Tags Interpretation-Clinician
// @Produce json
// @Param testee_id path string true "受试者ID"
// @Param assessment_id path string true "测评ID"
// @Success 200 {object} core.Response{data=response.ReportResponse}
// @Router /api/v1/clinicians/me/testees/{testee_id}/reports/{assessment_id} [get]
func (h *InterpretationClinicianHandler) Get(c *gin.Context) {
	org, user, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	testee, ok := parsePathUint(c, "testee_id", h.BaseHandler)
	if !ok {
		return
	}
	assessment, ok := parsePathUint(c, "assessment_id", h.BaseHandler)
	if !ok {
		return
	}
	result, err := h.service.GetParticipantReport(c.Request.Context(), interpretationclinician.Actor{OrgID: org, OperatorUserID: user}, interpretationclinician.GetQuery{TesteeID: testee, AssessmentID: assessment})
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, response.NewReportResponse(result))
}

type InterpretationOperationsHandler struct {
	*BaseHandler
	service interpretationoperations.Service
}

func NewInterpretationOperationsHandler(s interpretationoperations.Service) *InterpretationOperationsHandler {
	return &InterpretationOperationsHandler{BaseHandler: &BaseHandler{}, service: s}
}
func (h *InterpretationOperationsHandler) actor(c *gin.Context) (interpretationoperations.Actor, bool) {
	org, user, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return interpretationoperations.Actor{}, false
	}
	return interpretationoperations.Actor{OrgID: org, OperatorUserID: user}, true
}

// FindReport godoc
// @Summary 按 ReportID 查询 Interpretation 成品元数据
// @Tags Interpretation-Operations
// @Produce json
// @Param report_id path string true "报告ID"
// @Success 200 {object} core.Response{data=response.InterpretationReportWire}
// @Router /internal/v1/interpretation/reports/{report_id} [get]
func (h *InterpretationOperationsHandler) FindReport(c *gin.Context) {
	a, ok := h.actor(c)
	if !ok {
		return
	}
	id, ok := parseMetaPath(c, "report_id", h.BaseHandler)
	if !ok {
		return
	}
	v, err := h.service.FindReportByID(c.Request.Context(), a, id)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, v)
}

// FindOutcomeGenerations godoc
// @Summary 查询 Outcome 的报告生成历史
// @Tags Interpretation-Operations
// @Produce json
// @Param outcome_id path string true "OutcomeID"
// @Success 200 {object} core.Response{data=[]response.InterpretationGenerationWire}
// @Router /internal/v1/interpretation/outcomes/{outcome_id}/generations [get]
func (h *InterpretationOperationsHandler) FindOutcomeGenerations(c *gin.Context) {
	a, ok := h.actor(c)
	if !ok {
		return
	}
	id, ok := parseMetaPath(c, "outcome_id", h.BaseHandler)
	if !ok {
		return
	}
	v, err := h.service.FindGenerationsByOutcomeID(c.Request.Context(), a, id)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, v)
}

// FindOutcomeAdmissionFailures godoc
// @Summary 查询 Outcome 在创建 Generation 前的准入失败证据
// @Tags Interpretation-Operations
// @Produce json
// @Param outcome_id path string true "OutcomeID"
// @Success 200 {object} core.Response{data=[]interpretationoperations.AdmissionFailure}
// @Router /internal/v1/interpretation/outcomes/{outcome_id}/admission-failures [get]
func (h *InterpretationOperationsHandler) FindOutcomeAdmissionFailures(c *gin.Context) {
	a, ok := h.actor(c)
	if !ok {
		return
	}
	id, ok := parseMetaPath(c, "outcome_id", h.BaseHandler)
	if !ok {
		return
	}
	v, err := h.service.FindAdmissionFailuresByOutcomeID(c.Request.Context(), a, id)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, v)
}

// ListAdmissionFailures godoc
// @Summary 分页查询当前组织的 Interpretation 准入失败
// @Tags Interpretation-Operations
// @Produce json
// @Router /internal/v1/interpretation/admission-failures [get]
func (h *InterpretationOperationsHandler) ListAdmissionFailures(c *gin.Context) {
	a, ok := h.actor(c)
	if !ok {
		return
	}
	query := interpretationoperations.AdmissionFailureQuery{
		Decision: c.Query("decision"), Cursor: c.Query("cursor"),
	}
	if value := c.Query("reason"); value != "" {
		kind := admission.Kind(value)
		if !kind.IsValid() {
			h.Error(c, fmt.Errorf("invalid admission failure reason"))
			return
		}
		query.Kind = &kind
	}
	if value := c.Query("assessment_id"); value != "" {
		id, err := meta.ParseID(value)
		if err != nil || id.IsZero() {
			h.Error(c, fmt.Errorf("invalid assessment_id"))
			return
		}
		query.AssessmentID = &id
	}
	if value := c.Query("outcome_id"); value != "" {
		id, err := meta.ParseID(value)
		if err != nil || id.IsZero() {
			h.Error(c, fmt.Errorf("invalid outcome_id"))
			return
		}
		query.OutcomeID = &id
	}
	query.Limit, _ = strconv.Atoi(c.DefaultQuery("limit", "500"))
	page, err := h.service.ListAdmissionFailures(c.Request.Context(), a, query)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, page)
}

// FindAssessmentLifecycle godoc
// @Summary 查询 Assessment 的 Interpretation 生命周期
// @Tags Interpretation-Operations
// @Produce json
// @Param assessment_id path string true "测评ID"
// @Success 200 {object} core.Response{data=[]response.InterpretationGenerationWire}
// @Router /internal/v1/interpretation/assessments/{assessment_id}/lifecycle [get]
func (h *InterpretationOperationsHandler) FindAssessmentLifecycle(c *gin.Context) {
	a, ok := h.actor(c)
	if !ok {
		return
	}
	id, ok := parseMetaPath(c, "assessment_id", h.BaseHandler)
	if !ok {
		return
	}
	v, err := h.service.FindLifecycleByAssessmentID(c.Request.Context(), a, id)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, v)
}

// ListAssessmentReports godoc
// @Summary 查询 Assessment 的历史模板报告
// @Tags Interpretation-Operations
// @Produce json
// @Param assessment_id path string true "测评ID"
// @Success 200 {object} core.Response{data=[]response.InterpretationReportWire}
// @Router /internal/v1/interpretation/assessments/{assessment_id}/reports [get]
func (h *InterpretationOperationsHandler) ListAssessmentReports(c *gin.Context) {
	a, ok := h.actor(c)
	if !ok {
		return
	}
	id, ok := parseMetaPath(c, "assessment_id", h.BaseHandler)
	if !ok {
		return
	}
	v, err := h.service.ListHistoricalReportsByAssessmentID(c.Request.Context(), a, id)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, v)
}

func parsePathUint(c *gin.Context, name string, h *BaseHandler) (uint64, bool) {
	v, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil || v == 0 {
		h.BadRequestResponse(c, name+" is invalid", err)
		return 0, false
	}
	return v, true
}
func parseMetaPath(c *gin.Context, name string, h *BaseHandler) (meta.ID, bool) {
	v, ok := parsePathUint(c, name, h)
	return meta.FromUint64(v), ok
}
