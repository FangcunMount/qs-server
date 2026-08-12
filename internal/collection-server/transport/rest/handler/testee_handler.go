package handler

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	identityv2 "github.com/FangcunMount/iam/v3/api/grpc/iam/identity/v2"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/testee"
	"github.com/gin-gonic/gin"
)

type userProfileReader interface {
	IsEnabled() bool
	GetUserProfiles(ctx context.Context, userID string) (*identityv2.ListProfilesResponse, error)
}

// TesteeHandler 受试者处理器
type TesteeHandler struct {
	*BaseHandler
	testeeService      *testee.Service
	profileLinkService userProfileReader
}

// NewTesteeHandler 创建受试者处理器
func NewTesteeHandler(testeeService *testee.Service, profileLinkService userProfileReader) *TesteeHandler {
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

	profileIDs, err := loadUserProfileIDs(c.Request.Context(), h.profileLinkService, userID)
	if err != nil {
		h.InternalErrorResponse(c, "get user profiles from IAM failed", err)
		return
	}

	result, err := h.testeeService.ListMyTestees(c.Request.Context(), profileIDs, &req)
	if err != nil {
		h.InternalErrorResponse(c, "list my testees failed", err)
		return
	}

	h.Success(c, result)
}

func loadUserProfileIDs(ctx context.Context, reader userProfileReader, userID uint64) ([]uint64, error) {
	if reader == nil || !reader.IsEnabled() {
		return []uint64{}, nil
	}
	resp, err := reader.GetUserProfiles(ctx, strconv.FormatUint(userID, 10))
	if err != nil {
		return nil, fmt.Errorf("get user %d profiles: %w", userID, err)
	}
	if resp == nil {
		return nil, fmt.Errorf("get user %d profiles: IAM returned nil response", userID)
	}
	if resp.Total < 0 || int(resp.Total) != len(resp.Items) {
		return nil, fmt.Errorf("get user %d profiles: incomplete IAM response total=%d items=%d", userID, resp.Total, len(resp.Items))
	}

	profileIDs := make([]uint64, 0, len(resp.Items))
	for index, edge := range resp.Items {
		if edge == nil || edge.Profile == nil {
			return nil, fmt.Errorf("get user %d profiles: item %d is missing profile", userID, index)
		}
		rawID := strings.TrimSpace(edge.Profile.Id)
		profileID, parseErr := strconv.ParseUint(rawID, 10, 64)
		if parseErr != nil || profileID == 0 {
			return nil, fmt.Errorf("get user %d profiles: item %d has invalid profile id %q", userID, index, rawID)
		}
		profileIDs = append(profileIDs, profileID)
	}
	return profileIDs, nil
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
