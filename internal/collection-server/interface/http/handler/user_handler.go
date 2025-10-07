package handler

import (
	"net/http"

	"github.com/fangcun-mount/qs-server/internal/collection-server/application/user"
	"github.com/fangcun-mount/qs-server/internal/collection-server/infrastructure/auth"
	"github.com/fangcun-mount/qs-server/pkg/log"
	"github.com/gin-gonic/gin"
)

// UserHandler 用户处理器
type UserHandler struct {
	miniProgramRegistrar *user.MiniProgramRegistrar
	userQueryer          *user.UserQueryer
	jwtManager           *auth.JWTManager
}

// NewUserHandler 创建用户处理器
func NewUserHandler(
	miniProgramRegistrar *user.MiniProgramRegistrar,
	userQueryer *user.UserQueryer,
	jwtManager *auth.JWTManager,
) *UserHandler {
	return &UserHandler{
		miniProgramRegistrar: miniProgramRegistrar,
		userQueryer:          userQueryer,
		jwtManager:           jwtManager,
	}
}

// RegisterMiniProgram 小程序注册/登录
// @Summary 小程序注册/登录
// @Description 通过微信小程序 code 注册或登录
// @Tags 用户
// @Accept json
// @Produce json
// @Param request body user.RegisterRequest true "注册请求"
// @Success 200 {object} user.RegisterResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/users/miniprogram/register [post]
func (h *UserHandler) RegisterMiniProgram(c *gin.Context) {
	var req user.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Warnf("Invalid request body: %v", err)
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Code:    "INVALID_REQUEST",
			Message: "Invalid request body",
			Details: err.Error(),
		})
		return
	}

	resp, err := h.miniProgramRegistrar.Register(c.Request.Context(), &req)
	if err != nil {
		log.Errorf("Failed to register miniprogram user: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Code:    "REGISTER_FAILED",
			Message: "Failed to register user",
			Details: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// GetUser 获取用户信息
// @Summary 获取用户信息
// @Description 获取当前登录用户的信息
// @Tags 用户
// @Produce json
// @Security BearerAuth
// @Success 200 {object} user.UserInfo
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/users/me [get]
func (h *UserHandler) GetUser(c *gin.Context) {
	// 从上下文中获取用户 ID（由 JWT 中间件设置）
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Code:    "UNAUTHORIZED",
			Message: "User not authenticated",
		})
		return
	}

	userInfo, err := h.userQueryer.GetUser(c.Request.Context(), userID.(string))
	if err != nil {
		log.Errorf("Failed to get user info: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Code:    "GET_USER_FAILED",
			Message: "Failed to get user information",
			Details: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, userInfo)
}

// ErrorResponse 错误响应
type ErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}
