package handler

import (
	"net/http"
	"strconv"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/testee"
	"github.com/FangcunMount/qs-server/internal/collection-server/infra/iam"
	"github.com/FangcunMount/qs-server/internal/collection-server/interface/restful/middleware"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/pkg/core"
	"github.com/gin-gonic/gin"
)

// TesteeHandler 受试者处理器
type TesteeHandler struct {
	testeeService       *testee.Service
	guardianshipService *iam.GuardianshipService
}

// NewTesteeHandler 创建受试者处理器
func NewTesteeHandler(testeeService *testee.Service, guardianshipService *iam.GuardianshipService) *TesteeHandler {
	return &TesteeHandler{
		testeeService:       testeeService,
		guardianshipService: guardianshipService,
	}
}

// Create 创建受试者
// @Summary 创建受试者
// @Description 创建新的受试者信息
// @Tags 受试者
// @Accept json
// @Produce json
// @Param request body testee.CreateTesteeRequest true "受试者数据"
// @Success 200 {object} testee.TesteeResponse
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Security Bearer
// @Router /api/v1/testees [post]
func (h *TesteeHandler) Create(c *gin.Context) {
	var req testee.CreateTesteeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		core.WriteResponse(c, errors.WithCode(code.ErrBind, "bind request failed: %v", err), nil)
		return
	}

	// 验证用户是否已认证
	userID := middleware.GetUserID(c)
	if userID == 0 {
		core.WriteResponse(c, errors.WithCode(code.ErrTokenInvalid, "user not authenticated"), nil)
		return
	}

	result, err := h.testeeService.CreateTestee(c.Request.Context(), &req)
	if err != nil {
		core.WriteResponse(c, errors.WithCode(code.ErrDatabase, "create testee failed: %v", err), nil)
		return
	}

	c.JSON(http.StatusOK, result)
}

// Get 获取受试者详情
// @Summary 获取受试者详情
// @Description 根据ID获取受试者详细信息
// @Tags 受试者
// @Produce json
// @Param id path int true "受试者ID"
// @Success 200 {object} testee.TesteeResponse
// @Failure 400 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Security Bearer
// @Router /api/v1/testees/{id} [get]
func (h *TesteeHandler) Get(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		core.WriteResponse(c, errors.WithCode(code.ErrBind, "invalid id format"), nil)
		return
	}

	result, err := h.testeeService.GetTestee(c.Request.Context(), id)
	if err != nil {
		core.WriteResponse(c, errors.WithCode(code.ErrDatabase, "get testee failed: %v", err), nil)
		return
	}

	c.JSON(http.StatusOK, result)
}

// Update 更新受试者信息
// @Summary 更新受试者信息
// @Description 更新受试者的基本信息
// @Tags 受试者
// @Accept json
// @Produce json
// @Param id path int true "受试者ID"
// @Param request body testee.UpdateTesteeRequest true "更新数据"
// @Success 200 {object} testee.TesteeResponse
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Security Bearer
// @Router /api/v1/testees/{id} [put]
func (h *TesteeHandler) Update(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		core.WriteResponse(c, errors.WithCode(code.ErrBind, "invalid id format"), nil)
		return
	}

	var req testee.UpdateTesteeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		core.WriteResponse(c, errors.WithCode(code.ErrBind, "bind request failed: %v", err), nil)
		return
	}

	// 验证用户是否已认证
	userID := middleware.GetUserID(c)
	if userID == 0 {
		core.WriteResponse(c, errors.WithCode(code.ErrTokenInvalid, "user not authenticated"), nil)
		return
	}

	result, err := h.testeeService.UpdateTestee(c.Request.Context(), id, &req)
	if err != nil {
		core.WriteResponse(c, errors.WithCode(code.ErrDatabase, "update testee failed: %v", err), nil)
		return
	}

	c.JSON(http.StatusOK, result)
}

// List 查询当前用户的受试者列表
// @Summary 查询我的受试者列表
// @Description 查询当前用户（监护人）的所有受试者列表（支持分页）
// @Tags 受试者
// @Produce json
// @Param offset query int false "偏移量" default(0)
// @Param limit query int false "每页数量" default(20)
// @Success 200 {object} testee.ListTesteesResponse
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Security Bearer
// @Router /api/v1/testees [get]
func (h *TesteeHandler) List(c *gin.Context) {
	var req testee.ListTesteesRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		core.WriteResponse(c, errors.WithCode(code.ErrBind, "bind request failed: %v", err), nil)
		return
	}

	// 验证用户是否已认证
	userID := middleware.GetUserID(c)
	if userID == 0 {
		core.WriteResponse(c, errors.WithCode(code.ErrTokenInvalid, "user not authenticated"), nil)
		return
	}

	// 从 IAM SDK 获取当前用户的所有孩子ID列表
	childIDs := []uint64{}
	if h.guardianshipService != nil && h.guardianshipService.IsEnabled() {
		userIDStr := strconv.FormatUint(userID, 10)
		childrenResp, err := h.guardianshipService.GetUserChildren(c.Request.Context(), userIDStr)
		if err != nil {
			log.Warnf("Failed to get user children from IAM: %v, will return empty list", err)
			// 不返回错误，允许继续查询（返回空列表）
		} else if childrenResp != nil && len(childrenResp.Items) > 0 {
			for _, edge := range childrenResp.Items {
				if edge.Child != nil && edge.Child.Id != "" {
					if childID, err := strconv.ParseUint(edge.Child.Id, 10, 64); err == nil {
						childIDs = append(childIDs, childID)
					}
				}
			}
		}
	}

	result, err := h.testeeService.ListMyTestees(c.Request.Context(), childIDs, &req)
	if err != nil {
		core.WriteResponse(c, errors.WithCode(code.ErrDatabase, "list my testees failed: %v", err), nil)
		return
	}

	c.JSON(http.StatusOK, result)
}

// Exists 检查受试者是否存在
// @Summary 检查受试者是否存在
// @Description 根据IAM儿童ID检查受试者是否存在
// @Tags 受试者
// @Produce json
// @Param iam_child_id query int true "IAM儿童ID"
// @Success 200 {object} testee.TesteeExistsResponse
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Security Bearer
// @Router /api/v1/testees/exists [get]
func (h *TesteeHandler) Exists(c *gin.Context) {
	iamChildIDStr := c.Query("iam_child_id")

	iamChildID, err := strconv.ParseUint(iamChildIDStr, 10, 64)
	if err != nil {
		core.WriteResponse(c, errors.WithCode(code.ErrBind, "invalid iam_child_id format"), nil)
		return
	}

	// 验证用户是否已认证
	userID := middleware.GetUserID(c)
	if userID == 0 {
		core.WriteResponse(c, errors.WithCode(code.ErrTokenInvalid, "user not authenticated"), nil)
		return
	}

	result, err := h.testeeService.TesteeExists(c.Request.Context(), iamChildID)
	if err != nil {
		core.WriteResponse(c, errors.WithCode(code.ErrDatabase, "check testee existence failed: %v", err), nil)
		return
	}

	c.JSON(http.StatusOK, result)
}
