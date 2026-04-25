package handler

import (
	"context"
	"fmt"
	"strconv"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	actorAccessApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/access"
	clinicianApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/clinician"
	operatorApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/operator"
	testeeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee"
	domainRelation "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/relation"
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/request"
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/response"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/gin-gonic/gin"
)

// OperatorClinicianHandler 负责 staff / clinician / relation HTTP 入口。
type OperatorClinicianHandler struct {
	*BaseHandler
	operatorLifecycleService     operatorApp.OperatorLifecycleService
	operatorAuthorizationService operatorApp.OperatorAuthorizationService
	operatorQueryService         operatorApp.OperatorQueryService
	clinicianLifecycleService    clinicianApp.ClinicianLifecycleService
	clinicianQueryService        clinicianApp.ClinicianQueryService
	clinicianRelationshipService clinicianApp.ClinicianRelationshipService
	testeeQueryService           testeeApp.TesteeQueryService
	testeeAccessService          actorAccessApp.TesteeAccessService
}

func NewOperatorClinicianHandler(
	operatorLifecycleService operatorApp.OperatorLifecycleService,
	operatorAuthorizationService operatorApp.OperatorAuthorizationService,
	operatorQueryService operatorApp.OperatorQueryService,
	clinicianLifecycleService clinicianApp.ClinicianLifecycleService,
	clinicianQueryService clinicianApp.ClinicianQueryService,
	clinicianRelationshipService clinicianApp.ClinicianRelationshipService,
	testeeQueryService testeeApp.TesteeQueryService,
	testeeAccessService actorAccessApp.TesteeAccessService,
) *OperatorClinicianHandler {
	return &OperatorClinicianHandler{
		BaseHandler:                  NewBaseHandler(),
		operatorLifecycleService:     operatorLifecycleService,
		operatorAuthorizationService: operatorAuthorizationService,
		operatorQueryService:         operatorQueryService,
		clinicianLifecycleService:    clinicianLifecycleService,
		clinicianQueryService:        clinicianQueryService,
		clinicianRelationshipService: clinicianRelationshipService,
		testeeQueryService:           testeeQueryService,
		testeeAccessService:          testeeAccessService,
	}
}

func (h *OperatorClinicianHandler) CreateStaff(c *gin.Context) {
	var req request.CreateStaffRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.L(c.Request.Context()).Warnw("Invalid create staff request",
			"action", "create_staff",
			"resource", "staff",
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

	dto := toRegisterStaffDTO(&req, orgID)
	result, err := h.operatorLifecycleService.Register(c.Request.Context(), dto)
	if err != nil {
		logger.L(c.Request.Context()).Errorw("Failed to create staff",
			"action", "create_staff",
			"resource", "staff",
			"org_id", dto.OrgID,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	h.SuccessResponseWithMessage(c, "员工创建成功", toStaffResponse(result))
}

func (h *OperatorClinicianHandler) GetStaff(c *gin.Context) {
	orgID, err := h.RequireProtectedOrgID(c)
	if err != nil {
		h.Error(c, err)
		return
	}

	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		logger.L(c.Request.Context()).Warnw("Invalid staff ID",
			"action", "get_staff",
			"resource", "staff",
			"staff_id", idStr,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	result, err := h.operatorQueryService.GetByID(c.Request.Context(), id)
	if err != nil {
		logger.L(c.Request.Context()).Errorw("Failed to get staff",
			"action", "get_staff",
			"resource", "staff",
			"staff_id", id,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}
	if result.OrgID != orgID {
		h.Error(c, errors.WithCode(code.ErrPermissionDenied, "operator does not belong to current organization"))
		return
	}

	h.Success(c, toStaffResponse(result))
}

func (h *OperatorClinicianHandler) UpdateStaff(c *gin.Context) {
	orgID, err := h.RequireProtectedOrgID(c)
	if err != nil {
		h.Error(c, err)
		return
	}

	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		logger.L(c.Request.Context()).Warnw("Invalid staff ID",
			"action", "update_staff",
			"resource", "staff",
			"staff_id", idStr,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}
	current, err := h.loadProtectedStaff(c, orgID, id)
	if err != nil {
		h.Error(c, err)
		return
	}

	var req request.UpdateStaffRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.L(c.Request.Context()).Warnw("Invalid update staff request",
			"action", "update_staff",
			"resource", "staff",
			"staff_id", id,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}
	if err := h.updateStaffProfile(c, id, req); err != nil {
		h.Error(c, err)
		return
	}
	if err := h.syncStaffAuthorization(c, id, current, req); err != nil {
		h.Error(c, err)
		return
	}

	result, err := h.operatorQueryService.GetByID(c.Request.Context(), id)
	if err != nil {
		h.Error(c, err)
		return
	}

	h.SuccessResponseWithMessage(c, "员工更新成功", toStaffResponse(result))
}

func (h *OperatorClinicianHandler) DeleteStaff(c *gin.Context) {
	orgID, err := h.RequireProtectedOrgID(c)
	if err != nil {
		h.Error(c, err)
		return
	}

	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		logger.L(c.Request.Context()).Warnw("Invalid staff ID",
			"action", "delete_staff",
			"resource", "staff",
			"staff_id", idStr,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}
	result, err := h.operatorQueryService.GetByID(c.Request.Context(), id)
	if err != nil {
		h.Error(c, err)
		return
	}
	if result.OrgID != orgID {
		h.Error(c, errors.WithCode(code.ErrPermissionDenied, "operator does not belong to current organization"))
		return
	}

	if clinicianItem, err := h.clinicianQueryService.GetByOperator(c.Request.Context(), orgID, id); err == nil && clinicianItem != nil {
		h.Error(c, errors.WithCode(code.ErrValidation, "员工已绑定临床人员，请先解绑"))
		return
	} else if err != nil && !errors.IsCode(err, code.ErrUserNotFound) {
		h.Error(c, err)
		return
	}

	if err := h.operatorLifecycleService.Delete(c.Request.Context(), id); err != nil {
		logger.L(c.Request.Context()).Errorw("Failed to delete staff",
			"action", "delete_staff",
			"resource", "staff",
			"staff_id", id,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	h.SuccessResponseWithMessage(c, "员工删除成功", nil)
}

func (h *OperatorClinicianHandler) ListStaff(c *gin.Context) {
	var req request.ListStaffRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		logger.L(c.Request.Context()).Warnw("Invalid list staff request",
			"action", "list_staff",
			"resource", "staff",
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

	if req.Page == 0 {
		req.Page = 1
	}
	if req.PageSize == 0 {
		req.PageSize = 20
	}

	offset := (req.Page - 1) * req.PageSize
	listDTO := operatorApp.ListOperatorDTO{
		OrgID:  orgID,
		Role:   req.Role,
		Offset: offset,
		Limit:  req.PageSize,
	}

	listResult, err := h.operatorQueryService.ListOperators(c.Request.Context(), listDTO)
	if err != nil {
		logger.L(c.Request.Context()).Errorw("Failed to list staff",
			"action", "list_staff",
			"resource", "staff",
			"org_id", listDTO.OrgID,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	h.Success(c, toStaffListResponse(listResult.Items, listResult.TotalCount, req.Page, req.PageSize))
}

func (h *OperatorClinicianHandler) CreateClinician(c *gin.Context) {
	var req request.CreateClinicianRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Error(c, err)
		return
	}
	orgID, err := h.RequireProtectedOrgIDWithLegacy(c, req.OrgID)
	if err != nil {
		h.Error(c, err)
		return
	}

	result, err := h.clinicianLifecycleService.Register(c.Request.Context(), clinicianApp.RegisterClinicianDTO{
		OrgID:         orgID,
		OperatorID:    metaIDPtrToUint64(req.OperatorID),
		Name:          req.Name,
		Department:    req.Department,
		Title:         req.Title,
		ClinicianType: req.ClinicianType,
		EmployeeCode:  req.EmployeeCode,
		IsActive:      req.IsActive,
	})
	if err != nil {
		h.Error(c, err)
		return
	}

	h.SuccessResponseWithMessage(c, "从业者创建成功", toClinicianResponse(result))
}

func (h *OperatorClinicianHandler) UpdateClinician(c *gin.Context) {
	orgID, err := h.RequireProtectedOrgID(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		h.Error(c, err)
		return
	}
	if _, err := h.requireClinicianInOrg(c, orgID, id); err != nil {
		h.Error(c, err)
		return
	}

	var req request.UpdateClinicianRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Error(c, err)
		return
	}

	result, err := h.clinicianLifecycleService.Update(c.Request.Context(), clinicianApp.UpdateClinicianDTO{
		ClinicianID:   id,
		Name:          req.Name,
		Department:    req.Department,
		Title:         req.Title,
		ClinicianType: req.ClinicianType,
		EmployeeCode:  req.EmployeeCode,
	})
	if err != nil {
		h.Error(c, err)
		return
	}

	h.SuccessResponseWithMessage(c, "从业者更新成功", toClinicianResponse(result))
}

func (h *OperatorClinicianHandler) ActivateClinician(c *gin.Context) {
	result, err := h.changeClinicianState(c, "activate_clinician", "Clinician activated", h.clinicianLifecycleService.Activate)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.SuccessResponseWithMessage(c, "从业者已激活", toClinicianResponse(result))
}

func (h *OperatorClinicianHandler) DeactivateClinician(c *gin.Context) {
	result, err := h.changeClinicianState(c, "deactivate_clinician", "Clinician deactivated", h.clinicianLifecycleService.Deactivate)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.SuccessResponseWithMessage(c, "从业者已停用", toClinicianResponse(result))
}

func (h *OperatorClinicianHandler) BindClinicianOperator(c *gin.Context) {
	orgID, err := h.RequireProtectedOrgID(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		h.Error(c, err)
		return
	}
	if _, err := h.requireClinicianInOrg(c, orgID, id); err != nil {
		h.Error(c, err)
		return
	}

	var req request.BindClinicianOperatorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Error(c, err)
		return
	}
	operatorItem, err := h.operatorQueryService.GetByID(c.Request.Context(), req.OperatorID.Uint64())
	if err != nil {
		h.Error(c, err)
		return
	}
	if operatorItem.OrgID != orgID {
		h.Error(c, errors.WithCode(code.ErrPermissionDenied, "operator does not belong to current organization"))
		return
	}

	result, err := h.clinicianLifecycleService.BindOperator(c.Request.Context(), clinicianApp.BindClinicianOperatorDTO{
		ClinicianID: id,
		OperatorID:  req.OperatorID.Uint64(),
	})
	if err != nil {
		h.Error(c, err)
		return
	}
	h.SuccessResponseWithMessage(c, "从业者绑定员工成功", toClinicianResponse(result))
}

func (h *OperatorClinicianHandler) UnbindClinicianOperator(c *gin.Context) {
	orgID, err := h.RequireProtectedOrgID(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		h.Error(c, err)
		return
	}
	if _, err := h.requireClinicianInOrg(c, orgID, id); err != nil {
		h.Error(c, err)
		return
	}

	result, err := h.clinicianLifecycleService.UnbindOperator(c.Request.Context(), id)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.SuccessResponseWithMessage(c, "从业者解绑员工成功", toClinicianResponse(result))
}

func (h *OperatorClinicianHandler) GetClinician(c *gin.Context) {
	orgID, err := h.RequireProtectedOrgID(c)
	if err != nil {
		h.Error(c, err)
		return
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		h.Error(c, err)
		return
	}

	result, err := h.clinicianQueryService.GetByID(c.Request.Context(), id)
	if err != nil {
		h.Error(c, err)
		return
	}
	if result.OrgID != orgID {
		h.Error(c, errors.WithCode(code.ErrPermissionDenied, "clinician does not belong to current organization"))
		return
	}

	h.Success(c, toClinicianResponse(result))
}

func (h *OperatorClinicianHandler) ListClinicians(c *gin.Context) {
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

func (h *OperatorClinicianHandler) ListClinicianTestees(c *gin.Context) {
	orgID, err := h.RequireProtectedOrgID(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	clinicianID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		h.Error(c, err)
		return
	}
	if _, err := h.requireClinicianInOrg(c, orgID, clinicianID); err != nil {
		h.Error(c, err)
		return
	}

	page, pageSize := paginationFromContext(c)
	result, err := h.clinicianRelationshipService.ListAssignedTestees(c.Request.Context(), clinicianApp.ListAssignedTesteeDTO{
		OrgID:       orgID,
		ClinicianID: clinicianID,
		Offset:      (page - 1) * pageSize,
		Limit:       pageSize,
	})
	if err != nil {
		h.Error(c, err)
		return
	}

	items := make([]*response.TesteeResponse, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, toAssignedTesteeResponse(item))
	}
	totalPages := 0
	if pageSize > 0 {
		totalPages = int((result.TotalCount + int64(pageSize) - 1) / int64(pageSize))
	}
	h.Success(c, &response.TesteeListResponse{
		Items:      items,
		Total:      result.TotalCount,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	})
}

func (h *OperatorClinicianHandler) ListClinicianRelations(c *gin.Context) {
	orgID, err := h.RequireProtectedOrgID(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	clinicianID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		h.Error(c, err)
		return
	}
	if _, err := h.requireClinicianInOrg(c, orgID, clinicianID); err != nil {
		h.Error(c, err)
		return
	}

	h.listClinicianRelationsFor(c, orgID, clinicianID)
}

func (h *OperatorClinicianHandler) GetMyClinician(c *gin.Context) {
	clinicianItem, err := h.currentClinician(c)
	if err != nil {
		if errors.IsCode(err, code.ErrUserNotFound) {
			h.Success(c, nil)
			return
		}
		h.Error(c, err)
		return
	}

	h.Success(c, toClinicianResponse(clinicianItem))
}

func (h *OperatorClinicianHandler) ListMyClinicianTestees(c *gin.Context) {
	clinicianItem, err := h.currentClinician(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	_, operatorUserID, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return
	}

	page, pageSize := paginationFromContext(c)
	allowedTesteeIDs, err := h.testeeAccessService.ListAccessibleTesteeIDs(c.Request.Context(), clinicianItem.OrgID, operatorUserID)
	if err != nil {
		h.Error(c, err)
		return
	}

	result, err := h.testeeQueryService.ListTestees(c.Request.Context(), testeeApp.ListTesteeDTO{
		OrgID:                 clinicianItem.OrgID,
		AccessibleTesteeIDs:   allowedTesteeIDs,
		RestrictToAccessScope: true,
		Offset:                (page - 1) * pageSize,
		Limit:                 pageSize,
	})
	if err != nil {
		h.Error(c, err)
		return
	}

	h.Success(c, toTesteeListResponse(result.Items, result.TotalCount, page, pageSize))
}

func (h *OperatorClinicianHandler) ListMyClinicianRelations(c *gin.Context) {
	clinicianItem, err := h.currentClinician(c)
	if err != nil {
		h.Error(c, err)
		return
	}

	h.listClinicianRelationsFor(c, clinicianItem.OrgID, clinicianItem.ID)
}

func (h *OperatorClinicianHandler) AssignClinicianTestee(c *gin.Context) {
	var req request.AssignClinicianTesteeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Error(c, err)
		return
	}
	orgID, err := h.RequireProtectedOrgIDWithLegacy(c, req.OrgID)
	if err != nil {
		h.Error(c, err)
		return
	}

	result, err := h.clinicianRelationshipService.AssignTestee(c.Request.Context(), clinicianApp.AssignTesteeDTO{
		OrgID:        orgID,
		ClinicianID:  req.ClinicianID.Uint64(),
		TesteeID:     req.TesteeID.Uint64(),
		RelationType: req.RelationType,
		SourceType:   req.SourceType,
		SourceID:     metaIDPtrToUint64(req.SourceID),
	})
	if err != nil {
		h.Error(c, err)
		return
	}
	h.SuccessResponseWithMessage(c, "分配受试者成功", toRelationResponseFromClinicianResult(result))
}

func (h *OperatorClinicianHandler) AssignPrimaryClinicianTestee(c *gin.Context) {
	h.assignClinicianTesteeWithType(c, string(domainRelation.RelationTypePrimary), "设置主责从业者成功")
}

func (h *OperatorClinicianHandler) AssignAttendingClinicianTestee(c *gin.Context) {
	h.assignClinicianTesteeWithType(c, string(domainRelation.RelationTypeAttending), "设置跟进从业者成功")
}

func (h *OperatorClinicianHandler) AssignCollaboratorClinicianTestee(c *gin.Context) {
	h.assignClinicianTesteeWithType(c, string(domainRelation.RelationTypeCollaborator), "设置协作从业者成功")
}

func (h *OperatorClinicianHandler) TransferPrimaryClinicianTestee(c *gin.Context) {
	var req request.TransferPrimaryClinicianRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Error(c, err)
		return
	}
	orgID, err := h.RequireProtectedOrgIDWithLegacy(c, req.OrgID)
	if err != nil {
		h.Error(c, err)
		return
	}

	result, err := h.clinicianRelationshipService.TransferPrimary(c.Request.Context(), clinicianApp.TransferPrimaryDTO{
		OrgID:         orgID,
		ToClinicianID: req.ToClinicianID.Uint64(),
		TesteeID:      req.TesteeID.Uint64(),
		SourceType:    req.SourceType,
		SourceID:      metaIDPtrToUint64(req.SourceID),
	})
	if err != nil {
		h.Error(c, err)
		return
	}
	h.SuccessResponseWithMessage(c, "转移主责从业者成功", toRelationResponseFromClinicianResult(result))
}

func (h *OperatorClinicianHandler) UnbindClinicianTesteeRelation(c *gin.Context) {
	orgID, err := h.RequireProtectedOrgID(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	relationID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		h.Error(c, err)
		return
	}
	result, err := h.clinicianRelationshipService.UnbindRelation(c.Request.Context(), relationID)
	if err != nil {
		h.Error(c, err)
		return
	}
	if result.OrgID != orgID {
		h.Error(c, errors.WithCode(code.ErrPermissionDenied, "relation does not belong to current organization"))
		return
	}
	h.SuccessResponseWithMessage(c, "解绑成功", toRelationResponseFromClinicianResult(result))
}

func (h *OperatorClinicianHandler) GetTesteeClinicians(c *gin.Context) {
	h.listTesteeClinicianRelations(c, true)
}

func (h *OperatorClinicianHandler) ListTesteeClinicianRelations(c *gin.Context) {
	h.listTesteeClinicianRelations(c, false)
}

func (h *OperatorClinicianHandler) loadProtectedStaff(c *gin.Context, orgID int64, staffID uint64) (*operatorApp.OperatorResult, error) {
	current, err := h.operatorQueryService.GetByID(c.Request.Context(), staffID)
	if err != nil {
		return nil, err
	}
	if current.OrgID != orgID {
		return nil, errors.WithCode(code.ErrPermissionDenied, "operator does not belong to current organization")
	}
	return current, nil
}

func (h *OperatorClinicianHandler) updateStaffProfile(c *gin.Context, staffID uint64, req request.UpdateStaffRequest) error {
	_, err := h.operatorLifecycleService.UpdateProfile(c.Request.Context(), operatorApp.UpdateOperatorProfileDTO{
		OperatorID: staffID,
		Name:       req.Name,
		Email:      req.Email,
		Phone:      req.Phone,
	})
	return err
}

func (h *OperatorClinicianHandler) syncStaffAuthorization(c *gin.Context, staffID uint64, current *operatorApp.OperatorResult, req request.UpdateStaffRequest) error {
	targetActive := resolveTargetStaffActive(current.IsActive, req.IsActive)
	if err := h.syncStaffActiveState(c, staffID, current.IsActive, targetActive); err != nil {
		return err
	}
	if !targetActive || req.Roles == nil {
		return nil
	}

	latest, err := h.operatorQueryService.GetByID(c.Request.Context(), staffID)
	if err != nil {
		return err
	}
	return h.syncStaffRoles(c, staffID, latest.Roles, req.Roles)
}

func (h *OperatorClinicianHandler) syncStaffActiveState(c *gin.Context, staffID uint64, currentActive, targetActive bool) error {
	switch {
	case currentActive && !targetActive:
		return h.operatorAuthorizationService.Deactivate(c.Request.Context(), staffID)
	case !currentActive && targetActive:
		return h.operatorAuthorizationService.Activate(c.Request.Context(), staffID)
	default:
		return nil
	}
}

func (h *OperatorClinicianHandler) syncStaffRoles(c *gin.Context, staffID uint64, currentRoles, targetRoles []string) error {
	rolesToAssign, rolesToRemove := diffStringSet(currentRoles, targetRoles)
	for _, role := range rolesToAssign {
		if err := h.operatorAuthorizationService.AssignRole(c.Request.Context(), staffID, role); err != nil {
			return err
		}
	}
	for _, role := range rolesToRemove {
		if err := h.operatorAuthorizationService.RemoveRole(c.Request.Context(), staffID, role); err != nil {
			return err
		}
	}
	return nil
}

func (h *OperatorClinicianHandler) currentClinician(c *gin.Context) (*clinicianApp.ClinicianResult, error) {
	orgID, userID, err := h.RequireProtectedScope(c)
	if err != nil {
		return nil, err
	}

	operatorItem, err := h.operatorQueryService.GetByUser(c.Request.Context(), orgID, userID)
	if err != nil {
		logger.L(c.Request.Context()).Errorw("Failed to get operator for clinician",
			"action", "current_clinician",
			"org_id", orgID,
			"user_id", userID,
			"error", err.Error(),
		)
		return nil, err
	}
	if !operatorItem.IsActive {
		return nil, errors.WithCode(code.ErrPermissionDenied, "operator is inactive")
	}

	clinicianItem, err := h.clinicianQueryService.GetByOperator(c.Request.Context(), orgID, operatorItem.ID)
	if err != nil {
		return nil, err
	}
	if !clinicianItem.IsActive {
		return nil, errors.WithCode(code.ErrPermissionDenied, "clinician is inactive")
	}
	return clinicianItem, nil
}

func (h *OperatorClinicianHandler) requireClinicianInOrg(c *gin.Context, orgID int64, clinicianID uint64) (*clinicianApp.ClinicianResult, error) {
	return requireClinicianInOrg(c.Request.Context(), h.clinicianQueryService, orgID, clinicianID)
}

func requireClinicianInOrg(ctx context.Context, queryService clinicianApp.ClinicianQueryService, orgID int64, clinicianID uint64) (*clinicianApp.ClinicianResult, error) {
	if queryService == nil {
		return nil, errors.WithCode(code.ErrInternalServerError, "clinician query service not configured")
	}
	result, err := queryService.GetByID(ctx, clinicianID)
	if err != nil {
		return nil, err
	}
	if result.OrgID != orgID {
		return nil, errors.WithCode(code.ErrPermissionDenied, "clinician does not belong to current organization")
	}
	return result, nil
}

func (h *OperatorClinicianHandler) changeClinicianState(
	c *gin.Context,
	action string,
	logMessage string,
	change func(context.Context, uint64) (*clinicianApp.ClinicianResult, error),
) (*clinicianApp.ClinicianResult, error) {
	orgID, operatorUserID, err := h.RequireProtectedScope(c)
	if err != nil {
		return nil, err
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		return nil, err
	}
	if _, err := h.requireClinicianInOrg(c, orgID, id); err != nil {
		return nil, err
	}

	result, err := change(c.Request.Context(), id)
	if err != nil {
		return nil, err
	}
	logger.L(c.Request.Context()).Infow(logMessage,
		"action", action,
		"org_id", orgID,
		"clinician_id", id,
		"operator_user_id", operatorUserID,
	)
	return result, nil
}

func (h *OperatorClinicianHandler) listTesteeClinicianRelations(c *gin.Context, activeOnly bool) {
	testeeID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		h.Error(c, err)
		return
	}
	orgID, operatorUserID, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	if err := h.testeeAccessService.ValidateTesteeAccess(c.Request.Context(), orgID, operatorUserID, testeeID); err != nil {
		h.Error(c, err)
		return
	}
	result, err := h.clinicianRelationshipService.ListTesteeRelations(c.Request.Context(), clinicianApp.ListTesteeRelationDTO{
		OrgID:      orgID,
		TesteeID:   testeeID,
		ActiveOnly: activeOnly,
	})
	if err != nil {
		h.Error(c, err)
		return
	}

	items := make([]*response.TesteeClinicianRelationResponse, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, toTesteeClinicianRelationResponse(item))
	}
	h.Success(c, &response.TesteeClinicianRelationListResponse{Items: items})
}

func (h *OperatorClinicianHandler) listClinicianRelationsFor(c *gin.Context, orgID int64, clinicianID uint64) {
	page, pageSize := paginationFromContext(c)
	result, err := h.clinicianRelationshipService.ListClinicianRelations(c.Request.Context(), clinicianApp.ListClinicianRelationDTO{
		OrgID:       orgID,
		ClinicianID: clinicianID,
		Offset:      (page - 1) * pageSize,
		Limit:       pageSize,
		ActiveOnly:  true,
	})
	if err != nil {
		h.Error(c, err)
		return
	}

	items := make([]*response.ClinicianRelationResponse, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, toClinicianRelationResponse(item))
	}

	totalPages := 0
	if pageSize > 0 {
		totalPages = int((result.TotalCount + int64(pageSize) - 1) / int64(pageSize))
	}
	h.Success(c, &response.ClinicianRelationListResponse{
		Items:      items,
		Total:      result.TotalCount,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	})
}

func (h *OperatorClinicianHandler) assignClinicianTesteeWithType(c *gin.Context, relationType string, successMessage string) {
	var req request.AssignClinicianTesteeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Error(c, err)
		return
	}
	orgID, err := h.RequireProtectedOrgIDWithLegacy(c, req.OrgID)
	if err != nil {
		h.Error(c, err)
		return
	}

	dto := clinicianApp.AssignTesteeDTO{
		OrgID:        orgID,
		ClinicianID:  req.ClinicianID.Uint64(),
		TesteeID:     req.TesteeID.Uint64(),
		RelationType: relationType,
		SourceType:   req.SourceType,
		SourceID:     metaIDPtrToUint64(req.SourceID),
	}

	var result *clinicianApp.RelationResult
	switch relationType {
	case string(domainRelation.RelationTypePrimary):
		result, err = h.clinicianRelationshipService.AssignPrimary(c.Request.Context(), dto)
	case string(domainRelation.RelationTypeCollaborator):
		result, err = h.clinicianRelationshipService.AssignCollaborator(c.Request.Context(), dto)
	default:
		result, err = h.clinicianRelationshipService.AssignAttending(c.Request.Context(), dto)
	}
	if err != nil {
		h.Error(c, err)
		return
	}
	h.SuccessResponseWithMessage(c, successMessage, toRelationResponseFromClinicianResult(result))
}

func resolveTargetStaffActive(currentActive bool, requested *bool) bool {
	if requested == nil {
		return currentActive
	}
	return *requested
}

func diffStringSet(current, target []string) ([]string, []string) {
	currentSet := make(map[string]struct{}, len(current))
	targetSet := make(map[string]struct{}, len(target))
	for _, role := range current {
		currentSet[role] = struct{}{}
	}
	for _, role := range target {
		targetSet[role] = struct{}{}
	}

	toAssign := make([]string, 0, len(target))
	for _, role := range target {
		if _, exists := currentSet[role]; !exists {
			toAssign = append(toAssign, role)
		}
	}

	toRemove := make([]string, 0, len(current))
	for _, role := range current {
		if _, exists := targetSet[role]; !exists {
			toRemove = append(toRemove, role)
		}
	}
	return toAssign, toRemove
}

func toRegisterStaffDTO(req *request.CreateStaffRequest, orgID int64) operatorApp.RegisterOperatorDTO {
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	return operatorApp.RegisterOperatorDTO{
		OrgID:    orgID,
		UserID:   req.UserID.Int64(),
		Roles:    req.Roles,
		Name:     req.Name,
		Email:    req.Email,
		Phone:    req.Phone,
		Password: req.Password,
		IsActive: isActive,
	}
}

func toStaffResponse(result *operatorApp.OperatorResult) *response.StaffResponse {
	return &response.StaffResponse{
		ID:       fmt.Sprintf("%d", result.ID),
		OrgID:    fmt.Sprintf("%d", result.OrgID),
		UserID:   fmt.Sprintf("%d", result.UserID),
		Roles:    result.Roles,
		Name:     result.Name,
		Email:    result.Email,
		Phone:    result.Phone,
		IsActive: result.IsActive,
	}
}

func toStaffListResponse(results []*operatorApp.OperatorResult, total int64, page, pageSize int) *response.StaffListResponse {
	items := make([]*response.StaffResponse, 0, len(results))
	for _, result := range results {
		items = append(items, toStaffResponse(result))
	}

	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		totalPages++
	}

	return &response.StaffListResponse{
		Items:      items,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}
}

func toClinicianResponse(item *clinicianApp.ClinicianResult) *response.ClinicianResponse {
	if item == nil {
		return nil
	}

	var operatorID *string
	if item.OperatorID != nil {
		value := strconv.FormatUint(*item.OperatorID, 10)
		operatorID = &value
	}

	return &response.ClinicianResponse{
		ID:                   strconv.FormatUint(item.ID, 10),
		OrgID:                strconv.FormatInt(item.OrgID, 10),
		OperatorID:           operatorID,
		Name:                 item.Name,
		Department:           item.Department,
		Title:                item.Title,
		ClinicianType:        item.ClinicianType,
		ClinicianTypeLabel:   response.LabelForClinicianType(item.ClinicianType),
		EmployeeCode:         item.EmployeeCode,
		IsActive:             item.IsActive,
		IsActiveLabel:        map[bool]string{true: "启用", false: "停用"}[item.IsActive],
		AssignedTesteeCount:  item.AssignedTesteeCount,
		AssessmentEntryCount: item.AssessmentEntryCount,
	}
}

func toClinicianListResponse(result *clinicianApp.ClinicianListResult, page, pageSize int) *response.ClinicianListResponse {
	items := make([]*response.ClinicianResponse, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, toClinicianResponse(item))
	}

	totalPages := 0
	if pageSize > 0 {
		totalPages = int((result.TotalCount + int64(pageSize) - 1) / int64(pageSize))
	}

	return &response.ClinicianListResponse{
		Items:      items,
		Total:      result.TotalCount,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}
}

func toRelationResponseFromClinicianResult(item *clinicianApp.RelationResult) *response.RelationResponse {
	if item == nil {
		return nil
	}
	return buildRelationResponse(
		item.ID,
		item.OrgID,
		item.ClinicianID,
		item.TesteeID,
		item.RelationType,
		item.SourceType,
		item.SourceID,
		item.IsActive,
		item.BoundAt,
		item.UnboundAt,
	)
}

func toAssignedTesteeResponse(item *clinicianApp.AssignedTesteeResult) *response.TesteeResponse {
	if item == nil {
		return nil
	}
	return buildTesteeSummaryResponse(
		item.ID,
		item.OrgID,
		item.ProfileID,
		item.Name,
		item.Gender,
		item.Birthday,
		item.Tags,
		item.Source,
		item.IsKeyFocus,
	)
}

func toTesteeClinicianRelationResponse(item *clinicianApp.TesteeRelationResult) *response.TesteeClinicianRelationResponse {
	if item == nil {
		return nil
	}
	return &response.TesteeClinicianRelationResponse{
		Clinician: toClinicianResponse(item.Clinician),
		Relation:  toRelationResponseFromClinicianResult(item.Relation),
	}
}

func toClinicianRelationResponse(item *clinicianApp.ClinicianRelationResult) *response.ClinicianRelationResponse {
	if item == nil {
		return nil
	}
	return &response.ClinicianRelationResponse{
		Testee:   toAssignedTesteeResponse(item.Testee),
		Relation: toRelationResponseFromClinicianResult(item.Relation),
	}
}
