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

// OperatorClinicianHandler 负责 operator / clinician / relation HTTP 入口。
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

// CreateOperator 创建员工。
// @Summary 创建员工
// @Tags Operator
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param request body request.CreateOperatorRequest true "创建员工请求"
// @Success 200 {object} core.Response
// @Router /api/v1/operators [post]
func (h *OperatorClinicianHandler) CreateOperator(c *gin.Context) {
	var req request.CreateOperatorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.L(c.Request.Context()).Warnw("Invalid create operator request",
			"action", "create_operator",
			"resource", "operator",
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

	dto := toRegisterOperatorDTO(&req, orgID)
	result, err := h.operatorLifecycleService.Register(c.Request.Context(), dto)
	if err != nil {
		logger.L(c.Request.Context()).Errorw("Failed to create operator",
			"action", "create_operator",
			"resource", "operator",
			"org_id", dto.OrgID,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	h.SuccessResponseWithMessage(c, "员工创建成功", toOperatorResponse(result))
}

// GetOperator 获取员工详情。
// @Summary 获取员工详情
// @Tags Operator
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param id path int true "员工ID"
// @Success 200 {object} core.Response
// @Router /api/v1/operators/{id} [get]
func (h *OperatorClinicianHandler) GetOperator(c *gin.Context) {
	orgID, err := h.RequireProtectedOrgID(c)
	if err != nil {
		h.Error(c, err)
		return
	}

	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		logger.L(c.Request.Context()).Warnw("Invalid operator ID",
			"action", "get_operator",
			"resource", "operator",
			"operator_id", idStr,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	result, err := h.operatorQueryService.GetByID(c.Request.Context(), id)
	if err != nil {
		logger.L(c.Request.Context()).Errorw("Failed to get operator",
			"action", "get_operator",
			"resource", "operator",
			"operator_id", id,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}
	if result.OrgID != orgID {
		h.Error(c, errors.WithCode(code.ErrPermissionDenied, "operator does not belong to current organization"))
		return
	}

	h.Success(c, toOperatorResponse(result))
}

// UpdateOperator 更新员工。
// @Summary 更新员工
// @Tags Operator
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param id path int true "员工ID"
// @Param request body request.UpdateOperatorRequest true "更新员工请求"
// @Success 200 {object} core.Response
// @Router /api/v1/operators/{id} [put]
func (h *OperatorClinicianHandler) UpdateOperator(c *gin.Context) {
	orgID, err := h.RequireProtectedOrgID(c)
	if err != nil {
		h.Error(c, err)
		return
	}

	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		logger.L(c.Request.Context()).Warnw("Invalid operator ID",
			"action", "update_operator",
			"resource", "operator",
			"operator_id", idStr,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}
	current, err := h.loadProtectedOperator(c, orgID, id)
	if err != nil {
		h.Error(c, err)
		return
	}

	var req request.UpdateOperatorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.L(c.Request.Context()).Warnw("Invalid update operator request",
			"action", "update_operator",
			"resource", "operator",
			"operator_id", id,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}
	if err := h.updateOperatorProfile(c, id, req); err != nil {
		h.Error(c, err)
		return
	}
	if err := h.syncOperatorAuthorization(c, id, current, req); err != nil {
		h.Error(c, err)
		return
	}

	result, err := h.operatorQueryService.GetByID(c.Request.Context(), id)
	if err != nil {
		h.Error(c, err)
		return
	}

	h.SuccessResponseWithMessage(c, "员工更新成功", toOperatorResponse(result))
}

// ListOperator 查询员工列表。
// @Summary 查询员工列表
// @Tags Operator
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Success 200 {object} core.Response
// @Router /api/v1/operators [get]
func (h *OperatorClinicianHandler) ListOperator(c *gin.Context) {
	var req request.ListOperatorRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		logger.L(c.Request.Context()).Warnw("Invalid list operator request",
			"action", "list_operator",
			"resource", "operator",
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
		logger.L(c.Request.Context()).Errorw("Failed to list operator",
			"action", "list_operator",
			"resource", "operator",
			"org_id", listDTO.OrgID,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	h.Success(c, toOperatorListResponse(listResult.Items, listResult.TotalCount, req.Page, req.PageSize))
}

// CreateClinician 创建从业者。
// @Summary 创建从业者
// @Tags Clinician
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param request body request.CreateClinicianRequest true "创建从业者请求"
// @Success 200 {object} core.Response
// @Router /api/v1/clinicians [post]
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

// UpdateClinician 更新从业者。
// @Summary 更新从业者
// @Tags Clinician
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param id path int true "从业者ID"
// @Param request body request.UpdateClinicianRequest true "更新从业者请求"
// @Success 200 {object} core.Response
// @Router /api/v1/clinicians/{id} [put]
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

// ActivateClinician 激活从业者。
// @Summary 激活从业者
// @Tags Clinician
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param id path int true "从业者ID"
// @Success 200 {object} core.Response
// @Router /api/v1/clinicians/{id}/activate [post]
func (h *OperatorClinicianHandler) ActivateClinician(c *gin.Context) {
	result, err := h.changeClinicianState(c, "activate_clinician", "Clinician activated", h.clinicianLifecycleService.Activate)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.SuccessResponseWithMessage(c, "从业者已激活", toClinicianResponse(result))
}

// DeactivateClinician 停用从业者。
// @Summary 停用从业者
// @Tags Clinician
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param id path int true "从业者ID"
// @Success 200 {object} core.Response
// @Router /api/v1/clinicians/{id}/deactivate [post]
func (h *OperatorClinicianHandler) DeactivateClinician(c *gin.Context) {
	result, err := h.changeClinicianState(c, "deactivate_clinician", "Clinician deactivated", h.clinicianLifecycleService.Deactivate)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.SuccessResponseWithMessage(c, "从业者已停用", toClinicianResponse(result))
}

// GetClinician 获取从业者详情。
// @Summary 获取从业者详情
// @Tags Clinician
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param id path int true "从业者ID"
// @Success 200 {object} core.Response
// @Router /api/v1/clinicians/{id} [get]
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

// ListClinicians 查询从业者列表。
// @Summary 查询从业者列表
// @Tags Clinician
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Success 200 {object} core.Response
// @Param store_id query string false "服务门店 ID，与 unconfigured=true 互斥"
// @Param unconfigured query boolean false "仅查询未配置门店医生"
// @Router /api/v1/clinicians [get]
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
	if req.PageSize <= 0 || req.PageSize > 200 {
		h.BadRequestResponse(c, "page_size must be between 1 and 200", nil)
		return
	}
	orgID, err := h.RequireProtectedOrgIDWithLegacy(c, req.OrgID)
	if err != nil {
		h.Error(c, err)
		return
	}

	var storeID *uint64
	if raw, exists := c.GetQuery("store_id"); exists {
		v, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || v == 0 {
			h.Error(c, errors.WithCode(code.ErrInvalidArgument, "invalid store_id"))
			return
		}
		storeID = &v
	}
	unconfigured := false
	if raw, exists := c.GetQuery("unconfigured"); exists {
		v, err := strconv.ParseBool(raw)
		if err != nil {
			h.Error(c, errors.WithCode(code.ErrInvalidArgument, "invalid unconfigured filter"))
			return
		}
		unconfigured = v
	}
	result, err := h.clinicianQueryService.ListClinicians(c.Request.Context(), clinicianApp.ListClinicianDTO{
		StoreID: storeID, Unconfigured: unconfigured,
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

// ListClinicianTestees 查询从业者受试者列表。
// @Summary 查询从业者受试者列表
// @Tags Clinician
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param id path int true "从业者ID"
// @Success 200 {object} core.Response
// @Router /api/v1/clinicians/{id}/testees [get]
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

// ListClinicianRelations 查询从业者关系列表。
// @Summary 查询从业者关系列表
// @Tags Clinician
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param id path int true "从业者ID"
// @Success 200 {object} core.Response
// @Router /api/v1/clinicians/{id}/relations [get]
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

// AssignClinicianTestee 分配受试者。
// @Summary 分配受试者
// @Tags ClinicianRelation
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param request body request.AssignClinicianTesteeRequest true "分配请求"
// @Success 200 {object} core.Response
// @Router /api/v1/clinician-testee-relations/assign [post]
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

// AssignPrimaryClinicianTestee 设置主责从业者。
// @Summary 设置主责从业者
// @Tags ClinicianRelation
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param request body request.AssignClinicianTesteeRequest true "分配请求"
// @Success 200 {object} core.Response
// @Router /api/v1/clinician-testee-relations/assign-primary [post]
func (h *OperatorClinicianHandler) AssignPrimaryClinicianTestee(c *gin.Context) {
	h.assignClinicianTesteeWithType(c, string(domainRelation.RelationTypePrimary), "设置主责从业者成功")
}

// AssignAttendingClinicianTestee 设置跟进从业者。
// @Summary 设置跟进从业者
// @Tags ClinicianRelation
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param request body request.AssignClinicianTesteeRequest true "分配请求"
// @Success 200 {object} core.Response
// @Router /api/v1/clinician-testee-relations/assign-attending [post]
func (h *OperatorClinicianHandler) AssignAttendingClinicianTestee(c *gin.Context) {
	h.assignClinicianTesteeWithType(c, string(domainRelation.RelationTypeAttending), "设置跟进从业者成功")
}

// AssignCollaboratorClinicianTestee 设置协作从业者。
// @Summary 设置协作从业者
// @Tags ClinicianRelation
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param request body request.AssignClinicianTesteeRequest true "分配请求"
// @Success 200 {object} core.Response
// @Router /api/v1/clinician-testee-relations/assign-collaborator [post]
func (h *OperatorClinicianHandler) AssignCollaboratorClinicianTestee(c *gin.Context) {
	h.assignClinicianTesteeWithType(c, string(domainRelation.RelationTypeCollaborator), "设置协作从业者成功")
}

// TransferPrimaryClinicianTestee 转移主责从业者。
// @Summary 转移主责从业者
// @Tags ClinicianRelation
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param request body request.TransferPrimaryClinicianRequest true "转移请求"
// @Success 200 {object} core.Response
// @Router /api/v1/clinician-testee-relations/transfer-primary [post]
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

// UnbindClinicianTesteeRelation 解绑从业者受试者关系。
// @Summary 解绑从业者受试者关系
// @Tags ClinicianRelation
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param id path int true "关系ID"
// @Success 200 {object} core.Response
// @Router /api/v1/clinician-testee-relations/{id}/unbind [post]
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

// GetTesteeClinicians 获取受试者关联从业者。
// @Summary 获取受试者关联从业者
// @Tags 受试者
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param id path int true "受试者ID"
// @Success 200 {object} core.Response
// @Router /api/v1/testees/{id}/clinicians [get]
func (h *OperatorClinicianHandler) GetTesteeClinicians(c *gin.Context) {
	result, err := h.loadTesteeClinicianRelations(c, true)
	if err != nil {
		h.Error(c, err)
		return
	}

	items := make([]*response.ClinicianResponse, 0, len(result.Items))
	for _, item := range result.Items {
		if item == nil || item.Clinician == nil {
			continue
		}
		items = append(items, toClinicianResponse(item.Clinician))
	}
	h.Success(c, &response.ClinicianListResponse{Items: items})
}

// ListTesteeClinicianRelations 获取受试者从业者关系。
// @Summary 获取受试者从业者关系
// @Tags 受试者
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param id path int true "受试者ID"
// @Success 200 {object} core.Response
// @Router /api/v1/testees/{id}/clinician-relations [get]
func (h *OperatorClinicianHandler) ListTesteeClinicianRelations(c *gin.Context) {
	result, err := h.loadTesteeClinicianRelations(c, false)
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

func (h *OperatorClinicianHandler) loadProtectedOperator(c *gin.Context, orgID int64, operatorID uint64) (*operatorApp.OperatorResult, error) {
	current, err := h.operatorQueryService.GetByID(c.Request.Context(), operatorID)
	if err != nil {
		return nil, err
	}
	if current.OrgID != orgID {
		return nil, errors.WithCode(code.ErrPermissionDenied, "operator does not belong to current organization")
	}
	return current, nil
}

func (h *OperatorClinicianHandler) updateOperatorProfile(c *gin.Context, operatorID uint64, req request.UpdateOperatorRequest) error {
	_, err := h.operatorLifecycleService.UpdateProfile(c.Request.Context(), operatorApp.UpdateOperatorProfileDTO{
		OperatorID: operatorID,
		Name:       req.Name,
		Email:      req.Email,
		Phone:      req.Phone,
	})
	return err
}

func (h *OperatorClinicianHandler) syncOperatorAuthorization(c *gin.Context, operatorID uint64, current *operatorApp.OperatorResult, req request.UpdateOperatorRequest) error {
	targetActive := resolveTargetOperatorActive(current.IsActive, req.IsActive)
	if err := h.syncOperatorActiveState(c, operatorID, current.IsActive, targetActive); err != nil {
		return err
	}
	if !targetActive || req.Roles == nil {
		return nil
	}

	latest, err := h.operatorQueryService.GetByID(c.Request.Context(), operatorID)
	if err != nil {
		return err
	}
	return h.syncOperatorRoles(c, operatorID, latest.Roles, req.Roles)
}

func (h *OperatorClinicianHandler) syncOperatorActiveState(c *gin.Context, operatorID uint64, currentActive, targetActive bool) error {
	switch {
	case currentActive && !targetActive:
		return h.operatorAuthorizationService.Deactivate(c.Request.Context(), operatorID)
	case !currentActive && targetActive:
		return h.operatorAuthorizationService.Activate(c.Request.Context(), operatorID)
	default:
		return nil
	}
}

func (h *OperatorClinicianHandler) syncOperatorRoles(c *gin.Context, operatorID uint64, currentRoles, targetRoles []string) error {
	_ = currentRoles
	return h.operatorAuthorizationService.ReplaceRoles(c.Request.Context(), operatorID, targetRoles)
}

func (h *OperatorClinicianHandler) requireClinicianInOrg(c *gin.Context, orgID int64, clinicianID uint64) (*clinicianApp.ClinicianResult, error) {
	return requireClinicianInOrg(c.Request.Context(), h.clinicianQueryService, orgID, clinicianID)
}

func requireClinicianInOrg(ctx context.Context, queryService clinicianApp.ClinicianQueryService, orgID int64, clinicianID uint64) (*clinicianApp.ClinicianResult, error) {
	if queryService == nil {
		return nil, errors.WithCode(code.ErrInternalServerError, "clinician query service not configured")
	}
	result, err := queryService.GetBasicByID(ctx, clinicianID)
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

func (h *OperatorClinicianHandler) loadTesteeClinicianRelations(c *gin.Context, activeOnly bool) (*clinicianApp.TesteeRelationListResult, error) {
	testeeID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		return nil, err
	}
	orgID, operatorUserID, err := h.RequireProtectedScope(c)
	if err != nil {
		return nil, err
	}
	if err := h.testeeAccessService.ValidateTesteeAccess(c.Request.Context(), orgID, operatorUserID, testeeID); err != nil {
		return nil, err
	}
	result, err := h.clinicianRelationshipService.ListTesteeRelations(c.Request.Context(), clinicianApp.ListTesteeRelationDTO{
		OrgID:      orgID,
		TesteeID:   testeeID,
		ActiveOnly: activeOnly,
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		result = &clinicianApp.TesteeRelationListResult{}
	}
	return result, nil
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

func resolveTargetOperatorActive(currentActive bool, requested *bool) bool {
	if requested == nil {
		return currentActive
	}
	return *requested
}

func toRegisterOperatorDTO(req *request.CreateOperatorRequest, orgID int64) operatorApp.RegisterOperatorDTO {
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

func toOperatorResponse(result *operatorApp.OperatorResult) *response.OperatorResponse {
	return &response.OperatorResponse{
		Version:                result.Version,
		ID:                     fmt.Sprintf("%d", result.ID),
		OrgID:                  fmt.Sprintf("%d", result.OrgID),
		UserID:                 fmt.Sprintf("%d", result.UserID),
		Roles:                  result.Roles,
		EffectiveRoles:         result.EffectiveRoles,
		InheritedRoles:         result.InheritedRoles,
		AuthzPolicyVersion:     result.AuthzPolicyVersion,
		AuthzProjectionPending: result.AuthzProjectionPending,
		Name:                   result.Name,
		Email:                  result.Email,
		Phone:                  result.Phone,
		IsActive:               result.IsActive,
	}
}

func toOperatorListResponse(results []*operatorApp.OperatorResult, total int64, page, pageSize int) *response.OperatorListResponse {
	items := make([]*response.OperatorResponse, 0, len(results))
	for _, result := range results {
		items = append(items, toOperatorResponse(result))
	}

	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		totalPages++
	}

	return &response.OperatorListResponse{
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

	return &response.ClinicianResponse{
		StoreID: item.StoreID, StoreCode: item.StoreCode, StoreName: item.StoreName, StoreConfigured: item.StoreID != nil, Version: item.Version,
		ID:                   strconv.FormatUint(item.ID, 10),
		OrgID:                strconv.FormatInt(item.OrgID, 10),
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
	result := buildTesteeSummaryResponse(
		item.ID,
		item.OrgID,
		item.ProfileID,
		item.Name,
		item.Gender,
		item.Birthday,
		item.Source,
		item.IsKeyFocus,
	)
	if item.LastAssessmentAt != nil || item.TotalAssessments > 0 || item.LastRiskLevel != "" {
		result.AssessmentStats = &response.AssessmentStatsResponse{
			TotalCount:         item.TotalAssessments,
			LastAssessmentAt:   response.FormatDateTimePtr(item.LastAssessmentAt),
			LastRiskLevel:      item.LastRiskLevel,
			LastRiskLevelLabel: response.LabelForRiskLevel(item.LastRiskLevel),
		}
	}
	return result
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
