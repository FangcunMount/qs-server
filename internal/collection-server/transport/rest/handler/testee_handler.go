package handler

import (
	"strconv"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/testee"
	"github.com/FangcunMount/qs-server/internal/collection-server/infra/iam"
	"github.com/gin-gonic/gin"
)

// TesteeHandler 受试者处理器
type TesteeHandler struct {
	*BaseHandler
	testeeService      *testee.Service
	profileLinkService *iam.ProfileLinkService
}

// NewTesteeHandler 创建受试者处理器
func NewTesteeHandler(testeeService *testee.Service, profileLinkService *iam.ProfileLinkService) *TesteeHandler {
	return &TesteeHandler{
		BaseHandler:        NewBaseHandler(),
		testeeService:      testeeService,
		profileLinkService: profileLinkService,
	}
}

// Create 创建受试者
// @Summary 创建受试者
// @Description 创建新的受试者信息
// @Tags 受试者
// @Accept json
// @Produce json
// @Param request body testee.CreateTesteeRequest true "受试者数据"
// @Success 200 {object} core.Response{data=testee.TesteeResponse}
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Security BearerAuth
// @Router /api/v1/testees [post]
func (h *TesteeHandler) Create(c *gin.Context) {
	var req testee.CreateTesteeRequest
	if err := h.BindJSON(c, &req); err != nil {
		return
	}

	// 验证用户是否已认证
	userID := h.GetUserID(c)
	if userID == 0 {
		h.UnauthorizedResponse(c, "user not authenticated")
		return
	}

	result, err := h.testeeService.CreateTestee(c.Request.Context(), userID, &req)
	if err != nil {
		h.InternalErrorResponse(c, "create testee failed", err)
		return
	}

	h.Success(c, result)
}

// Get 获取受试者详情
// @Summary 获取受试者详情
// @Description 根据ID获取受试者详细信息
// @Tags 受试者
// @Produce json
// @Param id path int true "受试者ID"
// @Success 200 {object} core.Response{data=testee.TesteeResponse}
// @Failure 400 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Security BearerAuth
// @Router /api/v1/testees/{id} [get]
func (h *TesteeHandler) Get(c *gin.Context) {
	idStr := h.GetPathParam(c, "id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		h.BadRequestResponse(c, "invalid id format", err)
		return
	}

	result, err := h.testeeService.GetTestee(c.Request.Context(), id)
	if err != nil {
		h.InternalErrorResponse(c, "get testee failed", err)
		return
	}

	h.Success(c, result)
}

// GetCareContext 获取受试者照护上下文
// @Summary 获取受试者照护上下文
// @Description 获取当前受试者关联的临床人员和入口来源摘要
// @Tags 受试者
// @Produce json
// @Param id path int true "受试者ID"
// @Success 200 {object} core.Response{data=testee.TesteeCareContextResponse}
// @Failure 400 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Security BearerAuth
// @Router /api/v1/testees/{id}/care-context [get]
func (h *TesteeHandler) GetCareContext(c *gin.Context) {
	idStr := h.GetPathParam(c, "id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		h.BadRequestResponse(c, "invalid id format", err)
		return
	}

	result, err := h.testeeService.GetTesteeCareContext(c.Request.Context(), id)
	if err != nil {
		h.InternalErrorResponse(c, "get testee care context failed", err)
		return
	}

	h.Success(c, result)
}

// Update 更新受试者信息
// @Summary 更新受试者信息
// @Description 更新受试者的基本信息
// @Tags 受试者
// @Accept json
// @Produce json
// @Param id path int true "受试者ID"
// @Param request body testee.UpdateTesteeRequest true "更新数据"
// @Success 200 {object} core.Response{data=testee.TesteeResponse}
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Security BearerAuth
// @Router /api/v1/testees/{id} [put]
func (h *TesteeHandler) Update(c *gin.Context) {
	idStr := h.GetPathParam(c, "id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		h.BadRequestResponse(c, "invalid id format", err)
		return
	}

	var req testee.UpdateTesteeRequest
	if err := h.BindJSON(c, &req); err != nil {
		return
	}

	// 验证用户是否已认证
	userID := h.GetUserID(c)
	if userID == 0 {
		h.UnauthorizedResponse(c, "user not authenticated")
		return
	}

	result, err := h.testeeService.UpdateTestee(c.Request.Context(), id, &req)
	if err != nil {
		h.InternalErrorResponse(c, "update testee failed", err)
		return
	}

	h.Success(c, result)
}

// List 查询当前用户的受试者列表
// @Summary 查询我的受试者列表
// @Description 查询当前用户（监护人）的所有受试者列表（支持分页）
// @Tags 受试者
// @Produce json
// @Param offset query int false "偏移量" default(0)
// @Param limit query int false "每页数量" default(20)
// @Success 200 {object} core.Response{data=testee.ListTesteesResponse}
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Security BearerAuth
// @Router /api/v1/testees [get]
func (h *TesteeHandler) List(c *gin.Context) {
	var req testee.ListTesteesRequest
	if err := h.BindQuery(c, &req); err != nil {
		return
	}

	// 验证用户是否已认证
	userID := h.GetUserID(c)
	if userID == 0 {
		h.UnauthorizedResponse(c, "user not authenticated")
		return
	}

	// 从 IAM SDK 获取当前用户的所有 ProfileID 列表
	profileIDs := []uint64{}
	if h.profileLinkService != nil && h.profileLinkService.IsEnabled() {
		userIDStr := strconv.FormatUint(userID, 10)
		profilesResp, err := h.profileLinkService.GetUserProfiles(c.Request.Context(), userIDStr)
		if err != nil {
			log.Warnf("Failed to get user profiles from IAM: %v, will return empty list", err)
			// 不返回错误，允许继续查询（返回空列表）
		} else if profilesResp != nil && len(profilesResp.Items) > 0 {
			for _, edge := range profilesResp.Items {
				if edge.Profile != nil && edge.Profile.Id != "" {
					if profileID, err := strconv.ParseUint(edge.Profile.Id, 10, 64); err == nil {
						profileIDs = append(profileIDs, profileID)
					}
				}
			}
		}
	}

	result, err := h.testeeService.ListMyTestees(c.Request.Context(), profileIDs, &req)
	if err != nil {
		h.InternalErrorResponse(c, "list my testees failed", err)
		return
	}

	h.Success(c, result)
}

// Exists 检查受试者是否存在
// @Summary 检查受试者是否存在
// @Description 根据 IAM ProfileID 检查受试者是否存在
// @Tags 受试者
// @Produce json
// @Param iam_profile_id query string true "IAM档案ID"
// @Success 200 {object} core.Response{data=testee.TesteeExistsResponse}
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Security BearerAuth
// @Router /api/v1/testees/exists [get]
func (h *TesteeHandler) Exists(c *gin.Context) {
	iamProfileID := h.GetQueryParam(c, "iam_profile_id")
	if iamProfileID == "" {
		h.BadRequestResponse(c, "iam_profile_id is required", nil)
		return
	}

	// 验证用户是否已认证
	userID := h.GetUserID(c)
	if userID == 0 {
		h.UnauthorizedResponse(c, "user not authenticated")
		return
	}

	result, err := h.testeeService.TesteeExists(c.Request.Context(), iamProfileID)
	if err != nil {
		h.InternalErrorResponse(c, "check testee existence failed", err)
		return
	}

	h.Success(c, result)
}
