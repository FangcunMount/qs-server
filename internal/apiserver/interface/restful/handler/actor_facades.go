package handler

import (
	"fmt"
	"strconv"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	assessmentEntryApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/assessmententry"
	clinicianApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/clinician"
	"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/request"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/gin-gonic/gin"
)

type actorTesteeHTTP struct {
	handler *ActorHandler
}

type actorClinicianHTTP struct {
	handler *ActorHandler
}

type actorAssessmentEntryHTTP struct {
	handler *ActorHandler
}

func (h *ActorHandler) testeeHTTP() actorTesteeHTTP {
	return actorTesteeHTTP{handler: h}
}

func (h *ActorHandler) clinicianHTTP() actorClinicianHTTP {
	return actorClinicianHTTP{handler: h}
}

func (h *ActorHandler) assessmentEntryHTTP() actorAssessmentEntryHTTP {
	return actorAssessmentEntryHTTP{handler: h}
}

func (http actorTesteeHTTP) GetTestee(c *gin.Context) {
	h := http.handler
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		logger.L(c.Request.Context()).Warnw("Invalid testee ID",
			"action", "get_testee",
			"testee_id", idStr,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	orgID, _, err := h.validateProtectedTesteeAccess(c, id)
	if err != nil {
		h.Error(c, err)
		return
	}

	backendResult, err := h.testeeBackendQueryService.GetByIDWithGuardians(c.Request.Context(), id)
	if err != nil {
		logger.L(c.Request.Context()).Errorw("Failed to get testee with guardians",
			"action", "get_testee",
			"testee_id", id,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}
	if backendResult.OrgID != orgID {
		h.Error(c, errors.WithCode(code.ErrPermissionDenied, "testee does not belong to current organization"))
		return
	}

	h.Success(c, toTesteeBackendResponse(backendResult))
}

func (http actorTesteeHTTP) GetTesteeByProfileID(c *gin.Context) {
	h := http.handler
	_, operatorUserID, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return
	}

	var req request.GetTesteeByProfileIDRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		logger.L(c.Request.Context()).Warnw("Invalid get testee by profile_id request",
			"action", "get_testee_by_profile_id",
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}
	orgID, err := h.RequireProtectedOrgIDWithLegacy(c, req.OrgID)
	if err != nil {
		h.Error(c, err)
		return
	}

	profileIDStr := req.CanonicalProfileID()
	if profileIDStr == "" {
		h.BadRequestResponse(c, "profile_id is required", nil)
		return
	}

	testeeResult, err := h.fetchTesteeByProfile(c, orgID, profileIDStr)
	if err != nil {
		h.Error(c, err)
		return
	}
	if err := h.testeeAccessService.ValidateTesteeAccess(c.Request.Context(), orgID, operatorUserID, testeeResult.ID); err != nil {
		h.Error(c, err)
		return
	}

	if h.testeeBackendQueryService != nil {
		backendResult, backendErr := h.testeeBackendQueryService.GetByIDWithGuardians(c.Request.Context(), testeeResult.ID)
		if backendErr != nil {
			h.Error(c, backendErr)
			return
		}
		h.Success(c, toTesteeBackendResponse(backendResult))
		return
	}

	h.Success(c, toTesteeResponse(testeeResult))
}

func (http actorClinicianHTTP) ListClinicians(c *gin.Context) {
	h := http.handler
	req := request.ListClinicianRequest{Page: 1, PageSize: 20}
	if orgIDParam := c.Query("org_id"); orgIDParam != "" {
		if _, err := fmt.Sscan(orgIDParam, &req.OrgID); err != nil {
			h.Error(c, err)
			return
		}
	}
	if pageParam := c.Query("page"); pageParam != "" {
		if _, err := fmt.Sscan(pageParam, &req.Page); err != nil {
			h.Error(c, err)
			return
		}
	}
	if pageSizeParam := c.Query("page_size"); pageSizeParam != "" {
		if _, err := fmt.Sscan(pageSizeParam, &req.PageSize); err != nil {
			h.Error(c, err)
			return
		}
	}
	if req.Page <= 0 {
		h.BadRequestResponse(c, "page must be greater than 0", nil)
		return
	}
	if req.PageSize <= 0 || req.PageSize > 100 {
		h.BadRequestResponse(c, "page_size must be between 1 and 100", nil)
		return
	}
	orgID, err := h.RequireProtectedOrgIDWithLegacy(c, req.OrgID)
	if err != nil {
		h.Error(c, err)
		return
	}

	result, err := h.clinicianQueryService.ListClinicians(c.Request.Context(), clinicianApp.ListClinicianDTO{
		OrgID:  orgID,
		Offset: (req.Page - 1) * req.PageSize,
		Limit:  req.PageSize,
	})
	if err != nil {
		h.Error(c, err)
		return
	}

	h.Success(c, toClinicianListResponse(result, req.Page, req.PageSize))
}

func (http actorAssessmentEntryHTTP) ResolveAssessmentEntry(c *gin.Context) {
	h := http.handler
	result, err := h.assessmentEntryService.Resolve(c.Request.Context(), c.Param("token"))
	if err != nil {
		h.Error(c, err)
		return
	}

	h.Success(c, toAssessmentEntryResolvedResponse(result))
}

func (http actorAssessmentEntryHTTP) IntakeAssessmentEntry(c *gin.Context) {
	h := http.handler
	var req request.IntakeByAssessmentEntryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.BadRequestResponse(c, "invalid request body", nil)
		return
	}

	result, err := h.assessmentEntryService.Intake(c.Request.Context(), c.Param("token"), assessmentEntryApp.IntakeByAssessmentEntryDTO{
		ProfileID: req.ProfileID,
		Name:      req.Name,
		Gender:    parseGender(req.Gender),
		Birthday:  req.Birthday,
	})
	if err != nil {
		h.Error(c, err)
		return
	}

	h.SuccessResponseWithMessage(c, "扫码建档成功", toAssessmentEntryIntakeResponse(result))
}
