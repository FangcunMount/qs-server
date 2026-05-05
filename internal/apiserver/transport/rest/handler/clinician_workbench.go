package handler

import (
	"strconv"

	"github.com/FangcunMount/component-base/pkg/errors"
	workbenchApp "github.com/FangcunMount/qs-server/internal/apiserver/application/workbench"
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/response"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/gin-gonic/gin"
)

type ClinicianWorkbenchHandler struct {
	*BaseHandler
	service workbenchApp.Service
}

func NewClinicianWorkbenchHandler(service workbenchApp.Service) *ClinicianWorkbenchHandler {
	return &ClinicianWorkbenchHandler{
		BaseHandler: NewBaseHandler(),
		service:     service,
	}
}

// GetMyClinicianWorkbenchQueueSummary godoc
// @Summary 获取当前医生工作台队列统计
// @Description 返回当前医生名下高风险、复诊、重点关注队列数量。队列由最新测评风险、开放/逾期任务、重点关注字段动态生成，不以用户标签为事实来源。
// @Tags clinicians
// @Security BearerAuth
// @Produce json
// @Success 200 {object} response.ClinicianWorkbenchQueueSummaryResponse
// @Router /clinicians/me/workbench/queues/summary [get]
// @Router /practitioners/me/workbench/queues/summary [get]
func (h *ClinicianWorkbenchHandler) GetMyClinicianWorkbenchQueueSummary(c *gin.Context) {
	orgID, operatorUserID, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	result, err := h.service.GetSummary(c.Request.Context(), workbenchApp.Scope{
		Kind:           workbenchApp.ScopeKindClinicianMe,
		OrgID:          orgID,
		OperatorUserID: operatorUserID,
	})
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, response.NewClinicianWorkbenchQueueSummaryResponse(result))
}

// ListMyClinicianWorkbenchQueue godoc
// @Summary 获取当前医生工作台队列
// @Description queue_type 取值：high_risk、follow_up、key_focus。high_risk 使用最近一次有效测评风险，follow_up 返回每名受试者最紧急的开放或逾期任务，key_focus 使用重点关注字段。
// @Tags clinicians
// @Security BearerAuth
// @Produce json
// @Param queue_type path string true "队列类型：high_risk/follow_up/key_focus"
// @Param page query int false "页码，默认 1"
// @Param page_size query int false "每页数量，默认 20，最大 100"
// @Success 200 {object} response.ClinicianWorkbenchQueueResponse
// @Router /clinicians/me/workbench/queues/{queue_type} [get]
// @Router /practitioners/me/workbench/queues/{queue_type} [get]
func (h *ClinicianWorkbenchHandler) ListMyClinicianWorkbenchQueue(c *gin.Context) {
	orgID, operatorUserID, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	page, pageSize := paginationFromContext(c)
	result, err := h.service.ListQueue(c.Request.Context(), workbenchApp.ListQueueDTO{
		Scope: workbenchApp.Scope{
			Kind:           workbenchApp.ScopeKindClinicianMe,
			OrgID:          orgID,
			OperatorUserID: operatorUserID,
		},
		QueueType: workbenchApp.QueueType(c.Param("queue_type")),
		Page:      page,
		PageSize:  pageSize,
	})
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, response.NewClinicianWorkbenchQueueResponse(result))
}

// GetOrgWorkbenchQueueSummary godoc
// @Summary 获取管理员全院工作台队列统计
// @Description 返回当前机构高风险、复诊、重点关注队列数量；仅 qs:admin 可访问。clinician_id 可选，存在时限制到该医生已分配受试者。
// @Tags Workbench
// @Security BearerAuth
// @Produce json
// @Param clinician_id query int false "从业者 ID，可选"
// @Success 200 {object} response.ClinicianWorkbenchQueueSummaryResponse
// @Router /workbench/queues/summary [get]
func (h *ClinicianWorkbenchHandler) GetOrgWorkbenchQueueSummary(c *gin.Context) {
	orgID, err := h.RequireProtectedOrgID(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	clinicianID, err := optionalWorkbenchUint64Query(c, "clinician_id")
	if err != nil {
		h.Error(c, err)
		return
	}
	result, err := h.service.GetSummary(c.Request.Context(), workbenchApp.Scope{
		Kind:        workbenchApp.ScopeKindOrgAdmin,
		OrgID:       orgID,
		ClinicianID: clinicianID,
	})
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, response.NewClinicianWorkbenchQueueSummaryResponse(result))
}

// ListOrgWorkbenchQueue godoc
// @Summary 获取管理员全院工作台队列
// @Description queue_type 取值：high_risk、follow_up、key_focus；仅 qs:admin 可访问。clinician_id 可选，存在时限制到该医生已分配受试者。
// @Tags Workbench
// @Security BearerAuth
// @Produce json
// @Param queue_type path string true "队列类型：high_risk/follow_up/key_focus"
// @Param clinician_id query int false "从业者 ID，可选"
// @Param page query int false "页码，默认 1"
// @Param page_size query int false "每页数量，默认 20，最大 100"
// @Success 200 {object} response.ClinicianWorkbenchQueueResponse
// @Router /workbench/queues/{queue_type} [get]
func (h *ClinicianWorkbenchHandler) ListOrgWorkbenchQueue(c *gin.Context) {
	orgID, err := h.RequireProtectedOrgID(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	clinicianID, err := optionalWorkbenchUint64Query(c, "clinician_id")
	if err != nil {
		h.Error(c, err)
		return
	}
	page, pageSize := paginationFromContext(c)
	result, err := h.service.ListQueue(c.Request.Context(), workbenchApp.ListQueueDTO{
		Scope: workbenchApp.Scope{
			Kind:        workbenchApp.ScopeKindOrgAdmin,
			OrgID:       orgID,
			ClinicianID: clinicianID,
		},
		QueueType: workbenchApp.QueueType(c.Param("queue_type")),
		Page:      page,
		PageSize:  pageSize,
	})
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, response.NewClinicianWorkbenchQueueResponse(result))
}

func optionalWorkbenchUint64Query(c *gin.Context, key string) (*uint64, error) {
	raw := c.Query(key)
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || value == 0 {
		return nil, errors.WithCode(code.ErrInvalidArgument, "invalid %s: %s", key, raw)
	}
	return &value, nil
}
