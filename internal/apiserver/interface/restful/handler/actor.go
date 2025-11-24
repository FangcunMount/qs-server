package handler

import (
	"strconv"

	"github.com/FangcunMount/component-base/pkg/log"
	staffApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/staff"
	testeeShared "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee/shared"
	"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/request"
	"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/response"
	"github.com/gin-gonic/gin"
)

// ActorHandler Actor 模块的 HTTP Handler
type ActorHandler struct {
	*BaseHandler
	testeeService testeeShared.Service
	// Staff 服务按行为者组织
	staffLifecycleService     staffApp.StaffLifecycleService
	staffAuthorizationService staffApp.StaffAuthorizationService
	staffQueryService         staffApp.StaffQueryService
}

// NewActorHandler 创建 Actor Handler
func NewActorHandler(
	testeeService testeeShared.Service,
	staffLifecycleService staffApp.StaffLifecycleService,
	staffAuthorizationService staffApp.StaffAuthorizationService,
	staffQueryService staffApp.StaffQueryService,
) *ActorHandler {
	return &ActorHandler{
		BaseHandler:               NewBaseHandler(),
		testeeService:             testeeService,
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

	result, err := h.testeeService.GetByID(c.Request.Context(), id)
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

	dto := toUpdateTesteeDTO(&req)
	result, err := h.testeeService.Update(c.Request.Context(), id, dto)
	if err != nil {
		log.Errorf("Failed to update testee: %v", err)
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

	offset := (req.Page - 1) * req.PageSize

	// 根据查询条件调用不同的服务方法
	var results []*testeeShared.CompositeTesteeResult
	var total int64
	var err error

	if req.Name != "" {
		// 按姓名搜索
		results, err = h.testeeService.FindByName(c.Request.Context(), req.OrgID, req.Name)
		if err != nil {
			log.Errorf("Failed to find testees by name: %v", err)
			h.ErrorResponse(c, err)
			return
		}
		total = int64(len(results))
		// 手动分页
		start := offset
		end := offset + req.PageSize
		if start >= len(results) {
			results = []*testeeShared.CompositeTesteeResult{}
		} else {
			if end > len(results) {
				end = len(results)
			}
			results = results[start:end]
		}
	} else if len(req.Tags) > 0 {
		// 按标签搜索
		results, err = h.testeeService.ListByTags(c.Request.Context(), req.OrgID, req.Tags, offset, req.PageSize)
		if err != nil {
			log.Errorf("Failed to list testees by tags: %v", err)
			h.ErrorResponse(c, err)
			return
		}
		// 获取总数
		total, err = h.testeeService.CountByOrg(c.Request.Context(), req.OrgID)
		if err != nil {
			log.Errorf("Failed to count testees: %v", err)
			h.ErrorResponse(c, err)
			return
		}
	} else if req.IsKeyFocus != nil && *req.IsKeyFocus {
		// 查询重点关注
		results, err = h.testeeService.ListKeyFocus(c.Request.Context(), req.OrgID, offset, req.PageSize)
		if err != nil {
			log.Errorf("Failed to list key focus testees: %v", err)
			h.ErrorResponse(c, err)
			return
		}
		// 获取总数
		total, err = h.testeeService.CountByOrg(c.Request.Context(), req.OrgID)
		if err != nil {
			log.Errorf("Failed to count testees: %v", err)
			h.ErrorResponse(c, err)
			return
		}
	} else {
		// 查询机构下所有受试者
		results, err = h.testeeService.ListByOrg(c.Request.Context(), req.OrgID, offset, req.PageSize)
		if err != nil {
			log.Errorf("Failed to list testees: %v", err)
			h.ErrorResponse(c, err)
			return
		}
		// 获取总数
		total, err = h.testeeService.CountByOrg(c.Request.Context(), req.OrgID)
		if err != nil {
			log.Errorf("Failed to count testees: %v", err)
			h.ErrorResponse(c, err)
			return
		}
	}

	h.SuccessResponse(c, toTesteeListResponse(results, total, req.Page, req.PageSize))
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

// toCreateTesteeDTO 将创建请求转换为应用层 DTO
func toCreateTesteeDTO(req *request.CreateTesteeRequest) testeeShared.CreateTesteeDTO {
	var gender int8
	switch req.Gender {
	case "male", "男":
		gender = 1
	case "female", "女":
		gender = 2
	default:
		gender = 0
	}

	// 处理 ProfileID：优先使用 ProfileID，否则使用 IAMChildID（向后兼容）
	var profileID *uint64
	if req.ProfileID != nil {
		profileID = req.ProfileID
	} else if req.IAMChildID != nil {
		// 向后兼容：将 IAMChildID 转换为 ProfileID
		pid := uint64(*req.IAMChildID)
		profileID = &pid
	}

	return testeeShared.CreateTesteeDTO{
		OrgID:      req.OrgID,
		ProfileID:  profileID,
		Name:       req.Name,
		Gender:     gender,
		Birthday:   req.Birthday,
		Tags:       req.Tags,
		Source:     req.Source,
		IsKeyFocus: req.IsKeyFocus,
	}
}

// toUpdateTesteeDTO 将更新请求转换为应用层 DTO
func toUpdateTesteeDTO(req *request.UpdateTesteeRequest) testeeShared.UpdateTesteeDTO {
	dto := testeeShared.UpdateTesteeDTO{
		Name:       req.Name,
		Birthday:   req.Birthday,
		Tags:       req.Tags,
		IsKeyFocus: req.IsKeyFocus,
	}

	if req.Gender != nil {
		var gender int8
		switch *req.Gender {
		case "male", "男":
			gender = 1
		case "female", "女":
			gender = 2
		default:
			gender = 0
		}
		dto.Gender = &gender
	}

	return dto
}

// toTesteeResponse 将应用层结果转换为响应
func toTesteeResponse(result *testeeShared.CompositeTesteeResult) *response.TesteeResponse {
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

	if result.AssessmentStats != nil {
		resp.AssessmentStats = &response.AssessmentStatsResponse{
			TotalCount:       result.AssessmentStats.TotalAssessments,
			LastAssessmentAt: result.AssessmentStats.LastAssessmentAt,
			LastRiskLevel:    result.AssessmentStats.LastRiskLevel,
		}
	}

	return resp
}

// toTesteeListResponse 将应用层列表结果转换为响应
func toTesteeListResponse(results []*testeeShared.CompositeTesteeResult, total int64, page, pageSize int) *response.TesteeListResponse {
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
