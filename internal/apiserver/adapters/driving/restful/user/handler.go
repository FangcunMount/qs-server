package user

import (
	"github.com/asaskevich/govalidator"
	"github.com/gin-gonic/gin"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/adapters/driving/restful"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user/port"
)

// Handler 用户HTTP处理器
type Handler struct {
	restful.BaseHandler
	userService port.UserService
}

// NewHandler 创建用户处理器
func NewHandler(userService port.UserService) *Handler {
	return &Handler{
		userService: userService,
	}
}

// CreateUser 创建用户
// POST /api/v1/users
func (h *Handler) CreateUser(c *gin.Context) {
	var req port.UserCreateRequest
	if err := h.BindJSON(c, &req); err != nil {
		h.ErrorResponse(c, err)
		return
	}
	if ok, err := govalidator.ValidateStruct(req); !ok {
		h.ErrorResponse(c, err)
		return
	}

	// 创建用户
	userResponse, err := h.userService.CreateUser(c.Request.Context(), req)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, userResponse)
}

// GetUser 获取用户
// GET /api/v1/users/:id
func (h *Handler) GetUser(c *gin.Context) {
	var req port.UserIDRequest
	if err := h.BindQuery(c, &req); err != nil {
		h.ErrorResponse(c, err)
		return
	}
	if ok, err := govalidator.ValidateStruct(req); !ok {
		h.ErrorResponse(c, err)
		return
	}

	userResponse, err := h.userService.GetUser(c.Request.Context(), req)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, userResponse)
}

// UpdateUser 更新用户
// PUT /api/v1/users/:id
func (h *Handler) UpdateUser(c *gin.Context) {
	var req port.UserUpdateRequest
	if err := h.BindJSON(c, &req); err != nil {
		h.ErrorResponse(c, err)
		return
	}
	if ok, err := govalidator.ValidateStruct(req); !ok {
		h.ErrorResponse(c, err)
		return
	}

	userResponse, err := h.userService.UpdateUser(c.Request.Context(), req)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, userResponse)
}

// DeleteUser 删除用户
// DELETE /api/v1/users/:id
func (h *Handler) DeleteUser(c *gin.Context) {
	var req port.UserIDRequest
	if err := h.BindQuery(c, &req); err != nil {
		h.ErrorResponse(c, err)
		return
	}

	if err := h.userService.DeleteUser(c.Request.Context(), req); err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, nil)
}
