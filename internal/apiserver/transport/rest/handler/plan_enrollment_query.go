package handler

import (
	"strconv"

	"github.com/FangcunMount/component-base/pkg/errors"
	actoraccess "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/access"
	planapp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/gin-gonic/gin"
)

type PlanEnrollmentQueryHandler struct {
	*BaseHandler
	service planapp.EnrollmentQueryService
	access  actoraccess.TesteeAccessService
}

func NewPlanEnrollmentQueryHandler(service planapp.EnrollmentQueryService, access actoraccess.TesteeAccessService) *PlanEnrollmentQueryHandler {
	return &PlanEnrollmentQueryHandler{BaseHandler: NewBaseHandler(), service: service, access: access}
}

// List godoc
// @Summary 查询受试者 Plan Enrollment 轮次
// @Tags Plan-Enrollment
// @Param testee_id path uint64 true "受试者 ID"
// @Param plan_id query uint64 false "Plan ID"
// @Param status query string false "active/closed/terminated"
// @Success 200 {object} core.Response
// @Router /api/v2/plans/testees/{testee_id}/enrollments [get]
func (h *PlanEnrollmentQueryHandler) List(c *gin.Context) {
	orgID, userID, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	testeeID, err := strconv.ParseUint(c.Param("testee_id"), 10, 64)
	if err != nil || testeeID == 0 {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "invalid testee_id"))
		return
	}
	if h.access != nil {
		if err := h.access.ValidateTesteeAccess(c.Request.Context(), orgID, userID, testeeID); err != nil {
			h.Error(c, err)
			return
		}
	}
	query := planapp.EnrollmentQuery{OrgID: orgID, TesteeID: testeeID, Status: c.Query("status"), Page: 1, PageSize: 20}
	if query.Status != "" && query.Status != "active" && query.Status != "closed" && query.Status != "terminated" {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "invalid enrollment status"))
		return
	}
	if raw := c.Query("plan_id"); raw != "" {
		value, parseErr := strconv.ParseUint(raw, 10, 64)
		if parseErr != nil || value == 0 {
			h.Error(c, errors.WithCode(code.ErrInvalidArgument, "invalid plan_id"))
			return
		}
		query.PlanID = &value
	}
	if raw := c.Query("page"); raw != "" {
		query.Page, err = strconv.Atoi(raw)
		if err != nil {
			h.Error(c, errors.WithCode(code.ErrInvalidArgument, "invalid page"))
			return
		}
	}
	if raw := c.Query("page_size"); raw != "" {
		query.PageSize, err = strconv.Atoi(raw)
		if err != nil {
			h.Error(c, errors.WithCode(code.ErrInvalidArgument, "invalid page_size"))
			return
		}
	}
	value, err := h.service.ListEnrollments(c.Request.Context(), query)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, value)
}
