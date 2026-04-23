package handler

import (
	"context"
	"strconv"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	assessmentEntryApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/assessmententry"
	clinicianApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/clinician"
	testeeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee"
	domainRelation "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/relation"
	"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/request"
	"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/response"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/gin-gonic/gin"
)

func metaIDPtrToUint64(id *meta.ID) *uint64 {
	if id == nil || id.IsZero() {
		return nil
	}

	value := id.Uint64()
	return &value
}

func flexibleTimePtrToTimePtr(v *request.FlexibleTime) *time.Time {
	if v == nil || v.IsZero() {
		return nil
	}

	value := v.Time
	return &value
}

func (h *ActorHandler) generateAssessmentEntryQRCodeURL(ctx context.Context, token string) string {
	if h.qrCodeService == nil {
		return ""
	}

	generated, err := h.qrCodeService.GenerateAssessmentEntryQRCode(ctx, token)
	if err != nil {
		return ""
	}

	return generated
}

// CreateClinician 创建从业者。
// @Summary 创建从业者
// @Description 创建机构内从业者档案，仅 qs:admin 可访问；请求体中的 org_id 仅作兼容校验，实际以 JWT org_id 为准
// @Tags Actor-Clinician
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param request body request.CreateClinicianRequest true "创建从业者请求"
// @Success 200 {object} core.Response{data=response.ClinicianResponse}
// @Failure 429 {object} core.ErrResponse
// @Router /api/v1/clinicians [post]
func (h *ActorHandler) CreateClinician(c *gin.Context) {
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

func (h *ActorHandler) UpdateClinician(c *gin.Context) {
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

func (h *ActorHandler) ActivateClinician(c *gin.Context) {
	result, err := h.changeClinicianState(c, "activate_clinician", "Clinician activated", h.clinicianLifecycleService.Activate)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.SuccessResponseWithMessage(c, "从业者已激活", toClinicianResponse(result))
}

func (h *ActorHandler) DeactivateClinician(c *gin.Context) {
	result, err := h.changeClinicianState(c, "deactivate_clinician", "Clinician deactivated", h.clinicianLifecycleService.Deactivate)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.SuccessResponseWithMessage(c, "从业者已停用", toClinicianResponse(result))
}

func (h *ActorHandler) BindClinicianOperator(c *gin.Context) {
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

func (h *ActorHandler) UnbindClinicianOperator(c *gin.Context) {
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

// GetClinician 获取从业者详情。
// @Summary 获取从业者详情
// @Description 查询指定从业者详情，仅 qs:admin 可访问
// @Tags Actor-Clinician
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param id path string true "从业者ID"
// @Success 200 {object} core.Response{data=response.ClinicianResponse}
// @Failure 429 {object} core.ErrResponse
// @Router /api/v1/clinicians/{id} [get]
func (h *ActorHandler) GetClinician(c *gin.Context) {
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
// @Description 查询机构内从业者列表，仅 qs:admin 可访问；org_id 查询参数仅作兼容校验，实际以 JWT org_id 为准
// @Tags Actor-Clinician
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param org_id query int false "兼容字段：机构ID，若传入必须与 JWT org_id 一致"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} core.Response{data=response.ClinicianListResponse}
// @Failure 429 {object} core.ErrResponse
// @Router /api/v1/clinicians [get]
func (h *ActorHandler) ListClinicians(c *gin.Context) {
	h.clinicianHTTP().ListClinicians(c)
}

func (h *ActorHandler) ListClinicianTestees(c *gin.Context) {
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

func (h *ActorHandler) CreateClinicianAssessmentEntry(c *gin.Context) {
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

	var req request.CreateAssessmentEntryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Error(c, err)
		return
	}

	result, err := h.assessmentEntryService.Create(c.Request.Context(), assessmentEntryApp.CreateAssessmentEntryDTO{
		OrgID:         orgID,
		ClinicianID:   clinicianID,
		TargetType:    req.TargetType,
		TargetCode:    req.TargetCode,
		TargetVersion: req.TargetVersion,
		ExpiresAt:     flexibleTimePtrToTimePtr(req.ExpiresAt),
	})
	if err != nil {
		h.Error(c, err)
		return
	}

	qrCodeURL := h.generateAssessmentEntryQRCodeURL(c.Request.Context(), result.Token)
	h.SuccessResponseWithMessage(c, "测评入口创建成功", toAssessmentEntryResponse(result, qrCodeURL))
}

func (h *ActorHandler) ListClinicianAssessmentEntries(c *gin.Context) {
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
	result, err := h.assessmentEntryService.ListByClinician(c.Request.Context(), assessmentEntryApp.ListAssessmentEntryDTO{
		OrgID:       orgID,
		ClinicianID: clinicianID,
		Offset:      (page - 1) * pageSize,
		Limit:       pageSize,
	})
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, toAssessmentEntryListResponse(result, page, pageSize))
}

func (h *ActorHandler) ListClinicianRelations(c *gin.Context) {
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

// GetMyClinician 获取当前操作者对应的从业者。
// @Summary 获取我的从业者身份
// @Description 获取当前后台操作者绑定的从业者档案；当前也兼容旧的 /practitioners 路由别名
// @Tags Actor-Clinician
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Success 200 {object} core.Response{data=response.ClinicianResponse}
// @Failure 429 {object} core.ErrResponse
// @Router /api/v1/clinicians/me [get]
func (h *ActorHandler) GetMyClinician(c *gin.Context) {
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

// ListMyClinicianTestees 查询当前从业者名下受试者。
// @Summary 查询我的受试者
// @Description 查询当前从业者可访问的受试者列表，底层复用与 /api/v1/testees 相同的访问范围收口逻辑
// @Tags Actor-Clinician
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} core.Response{data=response.TesteeListResponse}
// @Failure 429 {object} core.ErrResponse
// @Router /api/v1/clinicians/me/testees [get]
func (h *ActorHandler) ListMyClinicianTestees(c *gin.Context) {
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

// CreateMyAssessmentEntry 创建当前从业者测评入口。
// @Summary 创建我的测评入口
// @Description 为当前从业者创建测评入口二维码；当前也兼容旧的 /practitioners 路由别名
// @Tags Actor-AssessmentEntry
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param request body request.CreateAssessmentEntryRequest true "创建测评入口请求"
// @Success 200 {object} core.Response{data=response.AssessmentEntryResponse}
// @Failure 429 {object} core.ErrResponse
// @Router /api/v1/clinicians/me/assessment-entries [post]
func (h *ActorHandler) CreateMyAssessmentEntry(c *gin.Context) {
	clinicianItem, err := h.currentClinician(c)
	if err != nil {
		h.Error(c, err)
		return
	}

	var req request.CreateAssessmentEntryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Error(c, err)
		return
	}

	result, err := h.assessmentEntryService.Create(c.Request.Context(), assessmentEntryApp.CreateAssessmentEntryDTO{
		OrgID:         clinicianItem.OrgID,
		ClinicianID:   clinicianItem.ID,
		TargetType:    req.TargetType,
		TargetCode:    req.TargetCode,
		TargetVersion: req.TargetVersion,
		ExpiresAt:     flexibleTimePtrToTimePtr(req.ExpiresAt),
	})
	if err != nil {
		h.Error(c, err)
		return
	}

	qrCodeURL := h.generateAssessmentEntryQRCodeURL(c.Request.Context(), result.Token)

	resp := toAssessmentEntryResponse(result, qrCodeURL)
	h.SuccessResponseWithMessage(c, "测评入口创建成功", resp)
}

// ListMyAssessmentEntries 查询当前从业者测评入口列表。
// @Summary 查询我的测评入口列表
// @Description 查询当前从业者创建的测评入口列表；当前也兼容旧的 /practitioners 路由别名
// @Tags Actor-AssessmentEntry
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} core.Response{data=response.AssessmentEntryListResponse}
// @Failure 429 {object} core.ErrResponse
// @Router /api/v1/clinicians/me/assessment-entries [get]
func (h *ActorHandler) ListMyAssessmentEntries(c *gin.Context) {
	clinicianItem, err := h.currentClinician(c)
	if err != nil {
		h.Error(c, err)
		return
	}

	page, pageSize := paginationFromContext(c)
	result, err := h.assessmentEntryService.ListByClinician(c.Request.Context(), assessmentEntryApp.ListAssessmentEntryDTO{
		OrgID:       clinicianItem.OrgID,
		ClinicianID: clinicianItem.ID,
		Offset:      (page - 1) * pageSize,
		Limit:       pageSize,
	})
	if err != nil {
		h.Error(c, err)
		return
	}

	h.Success(c, toAssessmentEntryListResponse(result, page, pageSize))
}

// GetMyAssessmentEntry 查询当前从业者测评入口详情。
// @Summary 查询我的测评入口详情
// @Description 查询当前从业者持有的单个测评入口详情
// @Tags Actor-AssessmentEntry
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param id path string true "入口ID"
// @Success 200 {object} core.Response{data=response.AssessmentEntryResponse}
// @Failure 429 {object} core.ErrResponse
// @Router /api/v1/clinicians/me/assessment-entries/{id} [get]
func (h *ActorHandler) GetMyAssessmentEntry(c *gin.Context) {
	clinicianItem, err := h.currentClinician(c)
	if err != nil {
		h.Error(c, err)
		return
	}

	entryID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		h.Error(c, err)
		return
	}

	result, err := h.assessmentEntryService.GetByID(c.Request.Context(), entryID)
	if err != nil {
		h.Error(c, err)
		return
	}
	if result.ClinicianID != clinicianItem.ID {
		h.Error(c, errors.WithCode(code.ErrPermissionDenied, "assessment entry does not belong to current clinician"))
		return
	}

	h.Success(c, toAssessmentEntryResponse(result, h.generateAssessmentEntryQRCodeURL(c.Request.Context(), result.Token)))
}

func (h *ActorHandler) ListMyClinicianRelations(c *gin.Context) {
	clinicianItem, err := h.currentClinician(c)
	if err != nil {
		h.Error(c, err)
		return
	}

	h.listClinicianRelationsFor(c, clinicianItem.OrgID, clinicianItem.ID)
}

func (h *ActorHandler) AssignClinicianTestee(c *gin.Context) {
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

func (h *ActorHandler) AssignPrimaryClinicianTestee(c *gin.Context) {
	h.assignClinicianTesteeWithType(c, string(domainRelation.RelationTypePrimary), "设置主责从业者成功")
}

func (h *ActorHandler) AssignAttendingClinicianTestee(c *gin.Context) {
	h.assignClinicianTesteeWithType(c, string(domainRelation.RelationTypeAttending), "设置跟进从业者成功")
}

func (h *ActorHandler) AssignCollaboratorClinicianTestee(c *gin.Context) {
	h.assignClinicianTesteeWithType(c, string(domainRelation.RelationTypeCollaborator), "设置协作从业者成功")
}

func (h *ActorHandler) TransferPrimaryClinicianTestee(c *gin.Context) {
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

func (h *ActorHandler) UnbindClinicianTesteeRelation(c *gin.Context) {
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

func (h *ActorHandler) GetTesteeClinicians(c *gin.Context) {
	h.listTesteeClinicianRelations(c, true)
}

func (h *ActorHandler) ListTesteeClinicianRelations(c *gin.Context) {
	h.listTesteeClinicianRelations(c, false)
}

func (h *ActorHandler) GetAssessmentEntry(c *gin.Context) {
	orgID, err := h.RequireProtectedOrgID(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	entryID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		h.Error(c, err)
		return
	}
	result, err := h.assessmentEntryService.GetByID(c.Request.Context(), entryID)
	if err != nil {
		h.Error(c, err)
		return
	}
	if result.OrgID != orgID {
		h.Error(c, errors.WithCode(code.ErrPermissionDenied, "assessment entry does not belong to current organization"))
		return
	}
	h.Success(c, toAssessmentEntryResponse(result, h.generateAssessmentEntryQRCodeURL(c.Request.Context(), result.Token)))
}

func (h *ActorHandler) DeactivateAssessmentEntry(c *gin.Context) {
	h.setAssessmentEntryActive(c, false)
}

func (h *ActorHandler) ReactivateAssessmentEntry(c *gin.Context) {
	h.setAssessmentEntryActive(c, true)
}

func (h *ActorHandler) DeactivateMyAssessmentEntry(c *gin.Context) {
	h.setMyAssessmentEntryActive(c, false)
}

func (h *ActorHandler) ReactivateMyAssessmentEntry(c *gin.Context) {
	h.setMyAssessmentEntryActive(c, true)
}

// ResolveAssessmentEntry 公开解析测评入口。
// @Summary 公开解析测评入口
// @Description 公开解析测评入口 token，返回入口配置和所属从业者摘要
// @Tags Actor-AssessmentEntry
// @Produce json
// @Param token path string true "测评入口Token"
// @Success 200 {object} core.Response{data=response.AssessmentEntryResolvedResponse}
// @Failure 429 {object} core.ErrResponse
// @Router /api/v1/public/assessment-entries/{token} [get]
func (h *ActorHandler) ResolveAssessmentEntry(c *gin.Context) {
	h.assessmentEntryHTTP().ResolveAssessmentEntry(c)
}

// IntakeAssessmentEntry 公开扫码 intake。
// @Summary 公开扫码建档
// @Description 通过测评入口 token 建立受试者并自动绑定从业者关系
// @Tags Actor-AssessmentEntry
// @Accept json
// @Produce json
// @Param token path string true "测评入口Token"
// @Param request body request.IntakeByAssessmentEntryRequest true "扫码 intake 请求"
// @Success 200 {object} core.Response{data=response.AssessmentEntryIntakeResponse}
// @Failure 429 {object} core.ErrResponse
// @Router /api/v1/public/assessment-entries/{token}/intake [post]
func (h *ActorHandler) IntakeAssessmentEntry(c *gin.Context) {
	h.assessmentEntryHTTP().IntakeAssessmentEntry(c)
}

func (h *ActorHandler) currentClinician(c *gin.Context) (*clinicianApp.ClinicianResult, error) {
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

func paginationFromContext(c *gin.Context) (int, int) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

func parseGender(value string) int8 {
	switch value {
	case "male", "男":
		return 1
	case "female", "女":
		return 2
	default:
		return 0
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

func toAssessmentEntryResponse(item *assessmentEntryApp.AssessmentEntryResult, qrCodeURL string) *response.AssessmentEntryResponse {
	if item == nil {
		return nil
	}

	return &response.AssessmentEntryResponse{
		ID:              strconv.FormatUint(item.ID, 10),
		OrgID:           strconv.FormatInt(item.OrgID, 10),
		ClinicianID:     strconv.FormatUint(item.ClinicianID, 10),
		Token:           item.Token,
		TargetType:      item.TargetType,
		TargetTypeLabel: response.LabelForTargetType(item.TargetType),
		TargetCode:      item.TargetCode,
		TargetVersion:   item.TargetVersion,
		IsActive:        item.IsActive,
		IsActiveLabel:   map[bool]string{true: "启用", false: "停用"}[item.IsActive],
		ExpiresAt:       response.FormatDateTimePtr(item.ExpiresAt),
		QRCodeURL:       qrCodeURL,
	}
}

func toAssessmentEntryListResponse(result *assessmentEntryApp.AssessmentEntryListResult, page, pageSize int) *response.AssessmentEntryListResponse {
	items := make([]*response.AssessmentEntryResponse, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, toAssessmentEntryResponse(item, ""))
	}

	totalPages := 0
	if pageSize > 0 {
		totalPages = int((result.TotalCount + int64(pageSize) - 1) / int64(pageSize))
	}

	return &response.AssessmentEntryListResponse{
		Items:      items,
		Total:      result.TotalCount,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}
}

func toClinicianSummaryResponse(item *assessmentEntryApp.ClinicianSummaryResult) *response.ClinicianSummaryResponse {
	if item == nil {
		return nil
	}

	var operatorID *string
	if item.OperatorID != nil {
		value := strconv.FormatUint(*item.OperatorID, 10)
		operatorID = &value
	}

	return &response.ClinicianSummaryResponse{
		ID:                 strconv.FormatUint(item.ID, 10),
		OperatorID:         operatorID,
		Name:               item.Name,
		Department:         item.Department,
		Title:              item.Title,
		ClinicianType:      item.ClinicianType,
		ClinicianTypeLabel: response.LabelForClinicianType(item.ClinicianType),
	}
}

func toAssessmentEntryResolvedResponse(item *assessmentEntryApp.ResolvedAssessmentEntryResult) *response.AssessmentEntryResolvedResponse {
	if item == nil {
		return nil
	}

	return &response.AssessmentEntryResolvedResponse{
		Entry:     toAssessmentEntryResponse(item.Entry, ""),
		Clinician: toClinicianSummaryResponse(item.Clinician),
	}
}

func toRelationResponse(item *assessmentEntryApp.RelationSummaryResult) *response.RelationResponse {
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

func toAssessmentEntryIntakeResponse(item *assessmentEntryApp.AssessmentEntryIntakeResult) *response.AssessmentEntryIntakeResponse {
	if item == nil {
		return nil
	}

	return &response.AssessmentEntryIntakeResponse{
		Entry:      toAssessmentEntryResponse(item.Entry, ""),
		Clinician:  toClinicianSummaryResponse(item.Clinician),
		Testee:     toTesteeSummaryResponse(item.Testee),
		Relation:   toRelationResponse(item.Relation),
		Assignment: toRelationResponse(item.Assignment),
	}
}

func toTesteeSummaryResponse(item *assessmentEntryApp.TesteeSummaryResult) *response.TesteeResponse {
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

func (h *ActorHandler) changeClinicianState(
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

func buildRelationResponse(
	id uint64,
	orgID int64,
	clinicianID uint64,
	testeeID uint64,
	relationType string,
	sourceType string,
	sourceID *uint64,
	isActive bool,
	boundAt time.Time,
	unboundAt *time.Time,
) *response.RelationResponse {
	return &response.RelationResponse{
		ID:                strconv.FormatUint(id, 10),
		OrgID:             strconv.FormatInt(orgID, 10),
		ClinicianID:       strconv.FormatUint(clinicianID, 10),
		TesteeID:          strconv.FormatUint(testeeID, 10),
		RelationType:      relationType,
		RelationTypeLabel: response.LabelForRelationType(relationType),
		SourceType:        sourceType,
		SourceTypeLabel:   response.LabelForRelationSource(sourceType),
		SourceID:          uint64StringPtr(sourceID),
		IsActive:          isActive,
		IsActiveLabel:     boolLabel(isActive, "有效", "失效"),
		BoundAt:           response.FormatDateTimeValue(boundAt),
		UnboundAt:         response.FormatDateTimePtr(unboundAt),
	}
}

func buildTesteeSummaryResponse(
	id uint64,
	orgID int64,
	profileID *uint64,
	name string,
	genderValue int8,
	birthday *time.Time,
	tags []string,
	source string,
	isKeyFocus bool,
) *response.TesteeResponse {
	gender := response.GenderCodeFromValue(genderValue)
	profileIDStr := uint64StringPtr(profileID)

	return &response.TesteeResponse{
		ID:              strconv.FormatUint(id, 10),
		OrgID:           strconv.FormatInt(orgID, 10),
		ProfileID:       profileIDStr,
		IAMChildID:      response.LegacyIAMChildIDAlias(profileIDStr),
		Name:            name,
		Gender:          gender,
		GenderLabel:     response.LabelForGender(gender),
		Birthday:        response.FormatDatePtr(birthday),
		Tags:            tags,
		TagsLabel:       response.LabelTags(tags),
		Source:          source,
		SourceLabel:     response.LabelForTesteeSource(source),
		IsKeyFocus:      isKeyFocus,
		IsKeyFocusLabel: response.LabelForKeyFocus(isKeyFocus),
	}
}

func uint64StringPtr(value *uint64) *string {
	if value == nil {
		return nil
	}
	text := strconv.FormatUint(*value, 10)
	return &text
}

func boolLabel(value bool, trueLabel, falseLabel string) string {
	if value {
		return trueLabel
	}
	return falseLabel
}

func (h *ActorHandler) requireClinicianInOrg(c *gin.Context, orgID int64, clinicianID uint64) (*clinicianApp.ClinicianResult, error) {
	result, err := h.clinicianQueryService.GetByID(c.Request.Context(), clinicianID)
	if err != nil {
		return nil, err
	}
	if result.OrgID != orgID {
		return nil, errors.WithCode(code.ErrPermissionDenied, "clinician does not belong to current organization")
	}
	return result, nil
}

func (h *ActorHandler) listTesteeClinicianRelations(c *gin.Context, activeOnly bool) {
	testeeID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		h.Error(c, err)
		return
	}
	orgID, _, err := h.validateProtectedTesteeAccess(c, testeeID)
	if err != nil {
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

func (h *ActorHandler) listClinicianRelationsFor(c *gin.Context, orgID int64, clinicianID uint64) {
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

func (h *ActorHandler) setAssessmentEntryActive(c *gin.Context, active bool) {
	orgID, operatorUserID, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	entryID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		h.Error(c, err)
		return
	}

	var result *assessmentEntryApp.AssessmentEntryResult
	if active {
		result, err = h.assessmentEntryService.Reactivate(c.Request.Context(), entryID)
	} else {
		result, err = h.assessmentEntryService.Deactivate(c.Request.Context(), entryID)
	}
	if err != nil {
		h.Error(c, err)
		return
	}
	if result.OrgID != orgID {
		h.Error(c, errors.WithCode(code.ErrPermissionDenied, "assessment entry does not belong to current organization"))
		return
	}
	logger.L(c.Request.Context()).Infow("Assessment entry lifecycle changed",
		"action", map[bool]string{true: "reactivate_assessment_entry", false: "deactivate_assessment_entry"}[active],
		"org_id", orgID,
		"assessment_entry_id", entryID,
		"clinician_id", result.ClinicianID,
		"operator_user_id", operatorUserID,
		"is_active", result.IsActive,
	)
	if active {
		h.SuccessResponseWithMessage(c, "测评入口已启用", toAssessmentEntryResponse(result, ""))
		return
	}
	h.SuccessResponseWithMessage(c, "测评入口已停用", toAssessmentEntryResponse(result, ""))
}

func (h *ActorHandler) setMyAssessmentEntryActive(c *gin.Context, active bool) {
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
	entryID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		h.Error(c, err)
		return
	}

	var result *assessmentEntryApp.AssessmentEntryResult
	if active {
		result, err = h.assessmentEntryService.Reactivate(c.Request.Context(), entryID)
	} else {
		result, err = h.assessmentEntryService.Deactivate(c.Request.Context(), entryID)
	}
	if err != nil {
		h.Error(c, err)
		return
	}
	if result.ClinicianID != clinicianItem.ID {
		h.Error(c, errors.WithCode(code.ErrPermissionDenied, "assessment entry does not belong to current clinician"))
		return
	}
	logger.L(c.Request.Context()).Infow("Assessment entry lifecycle changed",
		"action", map[bool]string{true: "reactivate_my_assessment_entry", false: "deactivate_my_assessment_entry"}[active],
		"org_id", clinicianItem.OrgID,
		"assessment_entry_id", entryID,
		"clinician_id", result.ClinicianID,
		"operator_user_id", operatorUserID,
		"is_active", result.IsActive,
	)
	if active {
		h.SuccessResponseWithMessage(c, "测评入口已启用", toAssessmentEntryResponse(result, ""))
		return
	}
	h.SuccessResponseWithMessage(c, "测评入口已停用", toAssessmentEntryResponse(result, ""))
}

func (h *ActorHandler) assignClinicianTesteeWithType(c *gin.Context, relationType string, successMessage string) {
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
