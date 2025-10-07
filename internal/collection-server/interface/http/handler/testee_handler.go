package handler

import (
	"net/http"

	"github.com/fangcun-mount/qs-server/internal/collection-server/application/user"
	"github.com/fangcun-mount/qs-server/pkg/log"
	"github.com/gin-gonic/gin"
)

// TesteeHandler 受试者处理器
type TesteeHandler struct {
	testeeRegistrar *user.TesteeRegistrar
	userQueryer     *user.UserQueryer
}

// NewTesteeHandler 创建受试者处理器
func NewTesteeHandler(
	testeeRegistrar *user.TesteeRegistrar,
	userQueryer *user.UserQueryer,
) *TesteeHandler {
	return &TesteeHandler{
		testeeRegistrar: testeeRegistrar,
		userQueryer:     userQueryer,
	}
}

// CreateTestee 创建受试者
// @Summary 创建受试者
// @Description 为当前登录用户创建受试者信息
// @Tags 受试者
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body user.CreateTesteeRequest true "创建受试者请求"
// @Success 200 {object} user.CreateTesteeResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/testees/register [post]
func (h *TesteeHandler) CreateTestee(c *gin.Context) {
	// 从上下文中获取用户 ID（由 JWT 中间件设置）
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Code:    "UNAUTHORIZED",
			Message: "User not authenticated",
		})
		return
	}

	var req user.CreateTesteeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Warnf("Invalid request body: %v", err)
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Code:    "INVALID_REQUEST",
			Message: "Invalid request body",
			Details: err.Error(),
		})
		return
	}

	resp, err := h.testeeRegistrar.CreateTestee(c.Request.Context(), userID.(string), &req)
	if err != nil {
		log.Errorf("Failed to create testee: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Code:    "CREATE_TESTEE_FAILED",
			Message: "Failed to create testee",
			Details: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// GetTestee 获取受试者信息
// @Summary 获取当前用户的受试者信息
// @Description 获取当前登录用户关联的受试者信息
// @Tags 受试者
// @Produce json
// @Security BearerAuth
// @Success 200 {object} user.TesteeInfo
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/testees/me [get]
func (h *TesteeHandler) GetTestee(c *gin.Context) {
	// 从上下文中获取用户 ID（由 JWT 中间件设置）
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Code:    "UNAUTHORIZED",
			Message: "User not authenticated",
		})
		return
	}

	testeeInfo, err := h.userQueryer.GetTestee(c.Request.Context(), userID.(string))
	if err != nil {
		log.Errorf("Failed to get testee info for user: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Code:    "GET_TESTEE_FAILED",
			Message: "Failed to get testee information",
			Details: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, testeeInfo)
}
