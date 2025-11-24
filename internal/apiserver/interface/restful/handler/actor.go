package handler

import (
	"strconv"

	"github.com/FangcunMount/component-base/pkg/log"
	staffApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/staff"
	testeeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee"
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
	testeeQueryService        testeeApp.TesteeQueryService
	// Staff 服务按行为者组织
	staffLifecycleService     staffApp.StaffLifecycleService
	staffAuthorizationService staffApp.StaffAuthorizationService
	staffQueryService         staffApp.StaffQueryService
}

// NewActorHandler 创建 Actor Handler
func NewActorHandler(
	testeeRegistrationService testeeApp.TesteeRegistrationService,
	testeeManagementService testeeApp.TesteeManagementService,
	testeeQueryService testeeApp.TesteeQueryService,
	staffLifecycleService staffApp.StaffLifecycleService,
	staffAuthorizationService staffApp.StaffAuthorizationService,
	staffQueryService staffApp.StaffQueryService,
) *ActorHandler {
	return &ActorHandler{
		BaseHandler:               NewBaseHandler(),
		testeeRegistrationService: testeeRegistrationService,
		testeeManagementService:   testeeManagementService,
		testeeQueryService:        testeeQueryService,
		staffLifecycleService:     staffLifecycleService,
		staffAuthorizationService: staffAuthorizationService,
		staffQueryService:         staffQueryService,
	}
}

// ========== Testee API ==========

// GetTestee 获取受试者详情
// @Summary 获取受试者详情
// @Tags Actor
// @Produce json
// @Param id path int true "受试者ID"
// @Success 200 {object} Response{data=response.TesteeResponse}
// @Router /api/v1/testees/{id} [get]
func (h *ActorHandler) GetTestee(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		log.Errorf("Invalid testee ID: %v", err)
		h.ErrorResponse(c, err)
		return
	}

	// 使用查询服务
	result, err := h.testeeQueryService.GetByID(c.Request.Context(), id)
	if err != nil {
		log.Errorf("Failed to get testee: %v", err)
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, toTesteeResponse(result))
}

// UpdateTestee 更新受试者
// @Summary 更新受试者
// @Tags Actor
// @Accept json
// @Produce json
// @Param id path int true "受试者ID"
// @Param body body request.UpdateTesteeRequest true "更新受试者请求"
// @Success 200 {object} Response{data=response.TesteeResponse}
// @Router /api/v1/testees/{id} [put]
func (h *ActorHandler) UpdateTestee(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		log.Errorf("Invalid testee ID: %v", err)
		h.ErrorResponse(c, err)
		return
	}

	var req request.UpdateTesteeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Errorf("Invalid request: %v", err)
		h.ErrorResponse(c, err)
		return
	}

	// 使用管理服务更新基本信息（B端员工操作）
	if (req.Name != nil && *req.Name != "") || req.Gender != nil || req.Birthday != nil {
		dto := toUpdateTesteeProfileDTO(id, &req)
		err = h.testeeManagementService.UpdateBasicInfo(c.Request.Context(), dto)
		if err != nil {
			log.Errorf("Failed to update testee profile: %v", err)
			h.ErrorResponse(c, err)
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
			log.Errorf("Failed to update key focus status: %v", err)
			h.ErrorResponse(c, err)
			return
		}
	}

	// 查询更新后的结果
	result, err := h.testeeQueryService.GetByID(c.Request.Context(), id)
	if err != nil {
		log.Errorf("Failed to get updated testee: %v", err)
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponseWithMessage(c, "受试者更新成功", toTesteeResponse(result))
}

// ListTestees 查询受试者列表
// @Summary 查询受试者列表
// @Tags Actor
// @Produce json
// @Param org_id query int true "机构ID"
// @Param name query string false "姓名（模糊匹配）"
// @Param is_key_focus query bool false "是否重点关注"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} Response{data=response.TesteeListResponse}
// @Router /api/v1/testees [get]
func (h *ActorHandler) ListTestees(c *gin.Context) {
	var req request.ListTesteeRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		log.Errorf("Invalid request: %v", err)
		h.ErrorResponse(c, err)
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
		log.Errorf("Failed to list testees: %v", err)
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, toTesteeListResponse(listResult.Items, listResult.TotalCount, req.Page, req.PageSize))
}

// ========== Staff API ==========

// CreateStaff 创建员工
// @Summary 创建员工
// @Tags Actor
// @Accept json
// @Produce json
// @Param body body request.CreateStaffRequest true "创建员工请求"
// @Success 200 {object} Response{data=response.StaffResponse}
// @Router /api/v1/staff [post]
func (h *ActorHandler) CreateStaff(c *gin.Context) {
	var req request.CreateStaffRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Errorf("Invalid request: %v", err)
		h.ErrorResponse(c, err)
		return
	}

	dto := toRegisterStaffDTO(&req)
	// 使用生命周期服务 - 服务于人事/行放部门
	result, err := h.staffLifecycleService.Register(c.Request.Context(), dto)
	if err != nil {
		log.Errorf("Failed to create staff: %v", err)
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponseWithMessage(c, "员工创建成功", toStaffResponse(result))
}

// GetStaff 获取员工详情
// @Summary 获取员工详情
// @Tags Actor
// @Produce json
// @Param id path int true "员工ID"
// @Success 200 {object} Response{data=response.StaffResponse}
// @Router /api/v1/staff/{id} [get]
func (h *ActorHandler) GetStaff(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		log.Errorf("Invalid staff ID: %v", err)
		h.ErrorResponse(c, err)
		return
	}

	// 使用查询服务
	result, err := h.staffQueryService.GetByID(c.Request.Context(), id)
	if err != nil {
		log.Errorf("Failed to get staff: %v", err)
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, toStaffResponse(result))
}

// DeleteStaff 删除员工
// @Summary 删除员工
// @Tags Actor
// @Produce json
// @Param id path int true "员工ID"
// @Success 200 {object} Response
// @Router /api/v1/staff/{id} [delete]
func (h *ActorHandler) DeleteStaff(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		log.Errorf("Invalid staff ID: %v", err)
		h.ErrorResponse(c, err)
		return
	}

	// 使用生命周期服务 - 服务于人事/行政部门
	if err := h.staffLifecycleService.Delete(c.Request.Context(), id); err != nil {
		log.Errorf("Failed to delete staff: %v", err)
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponseWithMessage(c, "员工删除成功", nil)
}

// ListStaff 查询员工列表
// @Summary 查询员工列表
// @Tags Actor
// @Produce json
// @Param org_id query int true "机构ID"
// @Param role query string false "角色筛选"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} Response{data=response.StaffListResponse}
// @Router /api/v1/staff [get]
func (h *ActorHandler) ListStaff(c *gin.Context) {
	var req request.ListStaffRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		log.Errorf("Invalid request: %v", err)
		h.ErrorResponse(c, err)
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
		log.Errorf("Failed to list staff: %v", err)
		h.ErrorResponse(c, err)
		return
	}

	results = listResult.Items
	total = listResult.TotalCount

	h.SuccessResponse(c, toStaffListResponse(results, total, req.Page, req.PageSize))
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

	resp := &response.TesteeResponse{
		ID:         result.ID,
		OrgID:      result.OrgID,
		ProfileID:  result.ProfileID,
		IAMChildID: result.ProfileID, // 向后兼容：使用 ProfileID 填充
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
		OrgID:  req.OrgID,
		UserID: req.UserID,
		Roles:  req.Roles,
		Name:   req.Name,
		Email:  req.Email,
		Phone:  req.Phone,
	}
}

// toStaffResponse 将应用层结果转换为响应
func toStaffResponse(result *staffApp.StaffResult) *response.StaffResponse {
	return &response.StaffResponse{
		ID:       result.ID,
		OrgID:    result.OrgID,
		UserID:   result.UserID,
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
