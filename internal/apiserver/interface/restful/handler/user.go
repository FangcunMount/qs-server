package handler

import (
	"github.com/asaskevich/govalidator"
	"github.com/gin-gonic/gin"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user/port"
	"github.com/yshujie/questionnaire-scale/internal/pkg/middleware"
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
	var req port.UserIDRequest
	if err := h.BindQuery(c, &req); err != nil {
		h.ErrorResponse(c, err)
		return
	}
	if ok, err := govalidator.ValidateStruct(req); !ok {
		h.ErrorResponse(c, err)
		return
	}

	userResponse, err := h.userQueryer.GetUser(c.Request.Context(), req)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, userResponse)
}

// GetUserProfile 获取用户资料
// GET /api/v1/users/profile
func (h *UserHandler) GetUserProfile(c *gin.Context) {
	username := c.GetString(middleware.UsernameKey)
	userResponse, err := h.userQueryer.GetUserByUsername(c.Request.Context(), username)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, userResponse)
}
