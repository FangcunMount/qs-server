package handler

import (
	"fmt"
	"strconv"

	"github.com/FangcunMount/component-base/pkg/logger"
	staffApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/staff"
	testeeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/request"
	"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/response"
	"github.com/gin-gonic/gin"
)

// ActorHandler Actor 模块的 HTTP Handler
type ActorHandler struct {
	*BaseHandler
	// Testee 服务按行为者组织
	testeeRegistrationService testeeApp.TesteeRegistrationService
	testeeManagementService   testeeApp.TesteeManagementService
	testeeQueryService        testeeApp.TesteeQueryService        // 通用查询服务（小程序、C端）
	testeeBackendQueryService testeeApp.TesteeBackendQueryService // 后台查询服务（包含家长信息）
	// Staff 服务按行为者组织
	staffLifecycleService     staffApp.StaffLifecycleService
	staffAuthorizationService staffApp.StaffAuthorizationService
	staffQueryService         staffApp.StaffQueryService
	// IAM 服务（可选）
	guardianshipService *iam.GuardianshipService
}

// NewActorHandler 创建 Actor Handler
func NewActorHandler(
	testeeRegistrationService testeeApp.TesteeRegistrationService,
	testeeManagementService testeeApp.TesteeManagementService,
	testeeQueryService testeeApp.TesteeQueryService,
	testeeBackendQueryService testeeApp.TesteeBackendQueryService,
	staffLifecycleService staffApp.StaffLifecycleService,
	staffAuthorizationService staffApp.StaffAuthorizationService,
	staffQueryService staffApp.StaffQueryService,
	guardianshipService *iam.GuardianshipService,
) *ActorHandler {
	return &ActorHandler{
		BaseHandler:               NewBaseHandler(),
		testeeRegistrationService: testeeRegistrationService,
		testeeManagementService:   testeeManagementService,
		testeeQueryService:        testeeQueryService,
		testeeBackendQueryService: testeeBackendQueryService,
		staffLifecycleService:     staffLifecycleService,
		staffAuthorizationService: staffAuthorizationService,
		staffQueryService:         staffQueryService,
		guardianshipService:       guardianshipService,
	}
}

// ========== Testee API ==========

// GetTestee 获取受试者详情
// @Summary 获取受试者详情
// @Tags Actor
// @Produce json
// @Param id path string true "受试者ID"
// @Success 200 {object} core.Response{data=response.TesteeResponse}
// @Router /api/v1/testees/{id} [get]
// GetTestee 获取受试者详情（后台管理接口，包含家长信息）
func (h *ActorHandler) GetTestee(c *gin.Context) {
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

	// 使用后台查询服务（包含家长信息）
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

	// 转换为响应对象（包含家长信息）
	resp := toTesteeBackendResponse(backendResult)

	h.Success(c, resp)
}

// GetScaleAnalysis
// @Summary 获取受试者量表分析结果
// @Tags Testee
// @Produce json
// @Param id path string true "Testee ID"
// @Success 200 {object} core.Response{data=response.ScaleAnalysisResponse}
// @Failure 400 {object} core.Response
// @Failure 404 {object} core.Response
// @Router /api/v1/testees/{id}/scale-analysis [get]
func (h *ActorHandler) GetScaleAnalysis(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		logger.L(c.Request.Context()).Warnw("Invalid testee ID",
			"action", "get_scale_analysis",
			"testee_id", idStr,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	// 验证受试者是否存在
	_, err = h.testeeQueryService.GetByID(c.Request.Context(), id)
	if err != nil {
		logger.L(c.Request.Context()).Errorw("Failed to get testee",
			"action", "get_scale_analysis",
			"testee_id", id,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	// TODO: 查询该受试者的所有测评记录，按量表分组
	// 当前返回空数据结构
	resp := &response.ScaleAnalysisResponse{
		Scales: []response.ScaleTrendResponse{},
	}

	h.Success(c, resp)
}

// GetPeriodicStats
// @Summary 获取受试者周期统计
// @Tags Testee
// @Produce json
// @Param id path string true "Testee ID"
// @Success 200 {object} core.Response{data=response.PeriodicStatsResponse}
// @Failure 400 {object} core.Response
// @Failure 404 {object} core.Response
// @Router /api/v1/testees/{id}/periodic-stats [get]
func (h *ActorHandler) GetPeriodicStats(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		logger.L(c.Request.Context()).Warnw("Invalid testee ID",
			"action", "get_periodic_stats",
			"testee_id", idStr,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	// 验证受试者是否存在
	_, err = h.testeeQueryService.GetByID(c.Request.Context(), id)
	if err != nil {
		logger.L(c.Request.Context()).Errorw("Failed to get testee",
			"action", "get_periodic_stats",
			"testee_id", id,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	// TODO: 查询该受试者参与的周期性测评项目
	// 当前返回空数据结构
	resp := &response.PeriodicStatsResponse{
		Projects:       []response.PeriodicProjectResponse{},
		TotalProjects:  0,
		ActiveProjects: 0,
	}

	h.Success(c, resp)
}

// UpdateTestee 更新受试者
// @Summary 更新受试者
// @Tags Actor
// @Accept json
// @Produce json
// @Param id path string true "受试者ID"
// @Param body body request.UpdateTesteeRequest true "更新受试者请求"
// @Success 200 {object} core.Response{data=response.TesteeResponse}
// @Router /api/v1/testees/{id} [put]
// UpdateTestee 更新受试者
func (h *ActorHandler) UpdateTestee(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		logger.L(c.Request.Context()).Warnw("Invalid testee ID",
			"action", "update_testee",
			"testee_id", idStr,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	var req request.UpdateTesteeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.L(c.Request.Context()).Warnw("Invalid request",
			"action", "update_testee",
			"testee_id", id,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	// 使用管理服务更新基本信息（B端员工操作）
	if (req.Name != nil && *req.Name != "") || req.Gender != nil || req.Birthday != nil {
		dto := toUpdateTesteeProfileDTO(id, &req)
		err = h.testeeManagementService.UpdateBasicInfo(c.Request.Context(), dto)
		if err != nil {
			logger.L(c.Request.Context()).Errorw("Failed to update testee profile",
				"action", "update_testee",
				"resource", "testee",
				"testee_id", id,
				"error", err.Error(),
			)
			h.Error(c, err)
			return
		}
	}

	// 更新标签（如果提供）
	// 注意：旧接口使用覆盖式更新，新服务使用增量操作
	// 这里简化处理，只在有标签时调用
	// TODO: 可能需要先获取现有标签进行比较，实现完整的覆盖逻辑

	// 更新重点关注状态
	if req.IsKeyFocus != nil {
		if *req.IsKeyFocus {
			err = h.testeeManagementService.MarkAsKeyFocus(c.Request.Context(), id)
		} else {
			err = h.testeeManagementService.UnmarkKeyFocus(c.Request.Context(), id)
		}
		if err != nil {
			logger.L(c.Request.Context()).Errorw("Failed to update key focus status",
				"action", "update_testee",
				"resource", "testee",
				"testee_id", id,
				"field", "is_key_focus",
				"error", err.Error(),
			)
			h.Error(c, err)
			return
		}
	}

	// 查询更新后的结果
	result, err := h.testeeQueryService.GetByID(c.Request.Context(), id)
	if err != nil {
		logger.L(c.Request.Context()).Errorw("Failed to get updated testee",
			"action", "update_testee",
			"resource", "testee",
			"testee_id", id,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	h.SuccessResponseWithMessage(c, "受试者更新成功", toTesteeResponse(result))
}

// ListTestees 查询受试者列表
// @Summary 查询受试者列表
// @Tags Actor
// @Produce json
// @Param org_id query string true "机构ID"
// @Param name query string false "姓名（模糊匹配）"
// @Param is_key_focus query bool false "是否重点关注"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} core.Response{data=response.TesteeListResponse}
// @Router /api/v1/testees [get]
func (h *ActorHandler) ListTestees(c *gin.Context) {
	var req request.ListTesteeRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		logger.L(c.Request.Context()).Warnw("Invalid list testees request",
			"action", "list_testees",
			"resource", "testee",
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	// 设置默认值
	if req.Page == 0 {
		req.Page = 1
	}
	if req.PageSize == 0 {
		req.PageSize = 20
	}

	// 使用查询服务 - 统一通过 ListTestees 方法处理所有查询
	dto := testeeApp.ListTesteeDTO{
		OrgID:    req.OrgID,
		Name:     req.Name,
		Tags:     req.Tags,
		KeyFocus: req.IsKeyFocus,
		Offset:   (req.Page - 1) * req.PageSize,
		Limit:    req.PageSize,
	}

	listResult, err := h.testeeQueryService.ListTestees(c.Request.Context(), dto)
	if err != nil {
		logger.L(c.Request.Context()).Errorw("Failed to list testees",
			"action", "list_testees",
			"resource", "testee",
			"org_id", dto.OrgID,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	h.Success(c, toTesteeListResponse(listResult.Items, listResult.TotalCount, req.Page, req.PageSize))
}

// ========== Staff API ==========

// CreateStaff 创建员工
// @Summary 创建员工
// @Tags Actor
// @Accept json
// @Produce json
// @Param body body request.CreateStaffRequest true "创建员工请求"
// @Success 200 {object} core.Response{data=response.StaffResponse}
// @Router /api/v1/staff [post]
func (h *ActorHandler) CreateStaff(c *gin.Context) {
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

	dto := toRegisterStaffDTO(&req)
	// 使用生命周期服务 - 服务于人事/行放部门
	result, err := h.staffLifecycleService.Register(c.Request.Context(), dto)
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

// GetStaff 获取员工详情
// @Summary 获取员工详情
// @Tags Actor
// @Produce json
// @Param id path string true "员工ID"
// @Success 200 {object} core.Response{data=response.StaffResponse}
// @Router /api/v1/staff/{id} [get]
func (h *ActorHandler) GetStaff(c *gin.Context) {
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

	// 使用查询服务
	result, err := h.staffQueryService.GetByID(c.Request.Context(), id)
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

	h.Success(c, toStaffResponse(result))
}

// DeleteStaff 删除员工
// @Summary 删除员工
// @Tags Actor
// @Produce json
// @Param id path string true "员工ID"
// @Success 200 {object} core.Response
// @Router /api/v1/staff/{id} [delete]
func (h *ActorHandler) DeleteStaff(c *gin.Context) {
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

	// 使用生命周期服务 - 服务于人事/行政部门
	if err := h.staffLifecycleService.Delete(c.Request.Context(), id); err != nil {
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

// ListStaff 查询员工列表
// @Summary 查询员工列表
// @Tags Actor
// @Produce json
// @Param org_id query string true "机构ID"
// @Param role query string false "角色筛选"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} core.Response{data=response.StaffListResponse}
// @Router /api/v1/staff [get]
func (h *ActorHandler) ListStaff(c *gin.Context) {
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

	// 设置默认值
	if req.Page == 0 {
		req.Page = 1
	}
	if req.PageSize == 0 {
		req.PageSize = 20
	}

	offset := (req.Page - 1) * req.PageSize

	// 根据查询条件调用不同的服务方法
	var results []*staffApp.StaffResult
	var total int64
	var err error

	// 使用查询服务
	listDTO := staffApp.ListStaffDTO{
		OrgID:  req.OrgID,
		Role:   req.Role,
		Offset: offset,
		Limit:  req.PageSize,
	}

	listResult, err := h.staffQueryService.ListStaffs(c.Request.Context(), listDTO)
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

	results = listResult.Items
	total = listResult.TotalCount

	h.Success(c, toStaffListResponse(results, total, req.Page, req.PageSize))
}

// ========== 映射辅助函数 ==========

// toUpdateTesteeProfileDTO 将更新请求转换为应用层 UpdateTesteeProfileDTO
func toUpdateTesteeProfileDTO(testeeID uint64, req *request.UpdateTesteeRequest) testeeApp.UpdateTesteeProfileDTO {
	var gender int8
	if req.Gender != nil {
		switch *req.Gender {
		case "male", "男":
			gender = 1
		case "female", "女":
			gender = 2
		default:
			gender = 0
		}
	}

	var name string
	if req.Name != nil {
		name = *req.Name
	}

	return testeeApp.UpdateTesteeProfileDTO{
		TesteeID: testeeID,
		Name:     name,
		Gender:   gender,
		Birthday: req.Birthday,
	}
}

// TODO: Create Testee API 已废弃，未来将通过 Registration Service 实现

// toTesteeResponse 将应用层结果转换为响应
func toTesteeResponse(result *testeeApp.TesteeResult) *response.TesteeResponse {
	var gender string
	switch result.Gender {
	case 1:
		gender = "male"
	case 2:
		gender = "female"
	default:
		gender = "unknown"
	}

	// 转换 ID 字段为字符串
	idStr := fmt.Sprintf("%d", result.ID)
	orgIDStr := fmt.Sprintf("%d", result.OrgID)
	var profileIDStr *string
	if result.ProfileID != nil {
		s := fmt.Sprintf("%d", *result.ProfileID)
		profileIDStr = &s
	}

	resp := &response.TesteeResponse{
		ID:         idStr,
		OrgID:      orgIDStr,
		ProfileID:  profileIDStr,
		IAMChildID: profileIDStr, // 向后兼容：使用 ProfileID 填充
		Name:       result.Name,
		Gender:     gender,
		Birthday:   result.Birthday,
		Tags:       result.Tags,
		Source:     result.Source,
		IsKeyFocus: result.IsKeyFocus,
	}

	// 测评统计信息
	if result.LastAssessmentAt != nil {
		resp.AssessmentStats = &response.AssessmentStatsResponse{
			TotalCount:       result.TotalAssessments,
			LastAssessmentAt: result.LastAssessmentAt,
			LastRiskLevel:    result.LastRiskLevel,
		}
	}

	return resp
}

// toTesteeBackendResponse 将后台查询结果转换为响应（包含家长信息）
func toTesteeBackendResponse(backendResult *testeeApp.TesteeBackendResult) *response.TesteeResponse {
	// 先转换基础信息
	resp := toTesteeResponse(backendResult.TesteeResult)

	// 添加家长信息
	if len(backendResult.Guardians) > 0 {
		resp.Guardians = make([]response.GuardianResponse, 0, len(backendResult.Guardians))
		for _, guardian := range backendResult.Guardians {
			resp.Guardians = append(resp.Guardians, response.GuardianResponse{
				Name:     guardian.Name,
				Relation: guardian.Relation,
				Phone:    guardian.Phone,
			})
		}
	}

	return resp
}

// toTesteeListResponse 将应用层列表结果转换为响应
func toTesteeListResponse(results []*testeeApp.TesteeResult, total int64, page, pageSize int) *response.TesteeListResponse {
	items := make([]*response.TesteeResponse, 0, len(results))
	for _, result := range results {
		items = append(items, toTesteeResponse(result))
	}

	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		totalPages++
	}

	return &response.TesteeListResponse{
		Items:      items,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}
}

// toRegisterStaffDTO 将创建请求转换为应用层 DTO
func toRegisterStaffDTO(req *request.CreateStaffRequest) staffApp.RegisterStaffDTO {
	return staffApp.RegisterStaffDTO{
		OrgID: req.OrgID,
		// UserID left zero; application service will create/resolve IAM user
		Roles: req.Roles,
		Name:  req.Name,
		Email: req.Email,
		Phone: req.Phone,
	}
}

// toStaffResponse 将应用层结果转换为响应
func toStaffResponse(result *staffApp.StaffResult) *response.StaffResponse {
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

// toStaffListResponse 将应用层列表结果转换为响应
func toStaffListResponse(results []*staffApp.StaffResult, total int64, page, pageSize int) *response.StaffListResponse {
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
