package handler

import (
	"strconv"

	interpretationclinician "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/clinician"
	interpretationoperations "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/operations"
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/response"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/gin-gonic/gin"
)

type InterpretationClinicianHandler struct {
	*BaseHandler
	service interpretationclinician.Service
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
