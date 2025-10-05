package handler

import (
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/gin-gonic/gin"

	"github.com/fangcun-mount/qs-server/internal/apiserver/domain/user/port"
	"github.com/fangcun-mount/qs-server/internal/apiserver/interface/restful/request"
	"github.com/fangcun-mount/qs-server/internal/apiserver/interface/restful/response"
	"github.com/fangcun-mount/qs-server/internal/pkg/middleware"
)

// UserHandler 用户HTTP处理器
type UserHandler struct {
	BaseHandler
	userCreator         port.UserCreator
	userQueryer         port.UserQueryer
	userEditor          port.UserEditor
	userActivator       port.UserActivator
	userPasswordChanger port.PasswordChanger
}

// NewUserHandler 创建用户处理器
func NewUserHandler(userCreator port.UserCreator, userQueryer port.UserQueryer, userEditor port.UserEditor, userActivator port.UserActivator, userPasswordChanger port.PasswordChanger) *UserHandler {
	return &UserHandler{
		userCreator:         userCreator,
		userQueryer:         userQueryer,
		userEditor:          userEditor,
		userActivator:       userActivator,
		userPasswordChanger: userPasswordChanger,
	}
}

// GetUser 获取用户
// GET /api/v1/users/:id
func (h *UserHandler) GetUser(c *gin.Context) {
	var req request.UserIDRequest
	if err := h.BindQuery(c, &req); err != nil {
		h.ErrorResponse(c, err)
		return
	}
	if ok, err := govalidator.ValidateStruct(req); !ok {
		h.ErrorResponse(c, err)
		return
	}

	// 调用领域服务
	user, err := h.userQueryer.GetUser(c.Request.Context(), req.ID)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	// 转换为DTO响应
	response := response.UserResponse{
		ID:           user.ID().Value(),
		Username:     user.Username(),
		Nickname:     user.Nickname(),
		Phone:        user.Phone(),
		Avatar:       user.Avatar(),
		Introduction: user.Introduction(),
		Email:        user.Email(),
		Status:       user.Status().String(),
		CreatedAt:    user.CreatedAt().Format(time.RFC3339),
		UpdatedAt:    user.UpdatedAt().Format(time.RFC3339),
	}

	h.SuccessResponse(c, response)
}

// GetUserProfile 获取用户资料
// GET /api/v1/users/profile
func (h *UserHandler) GetUserProfile(c *gin.Context) {
	username := c.GetString(middleware.UsernameKey)

	// 调用领域服务
	user, err := h.userQueryer.GetUserByUsername(c.Request.Context(), username)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	// 转换为DTO响应
	response := response.UserResponse{
		ID:           user.ID().Value(),
		Username:     user.Username(),
		Nickname:     user.Nickname(),
		Phone:        user.Phone(),
		Avatar:       user.Avatar(),
		Introduction: user.Introduction(),
		Email:        user.Email(),
		Status:       user.Status().String(),
		CreatedAt:    user.CreatedAt().Format(time.RFC3339),
		UpdatedAt:    user.UpdatedAt().Format(time.RFC3339),
	}

	h.SuccessResponse(c, response)
}
