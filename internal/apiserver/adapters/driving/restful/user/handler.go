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
	userCreator         port.UserCreator
	userQueryer         port.UserQueryer
	userEditor          port.UserEditor
	userActivator       port.UserActivator
	userPasswordChanger port.PasswordChanger
}

// NewHandler 创建用户处理器
func NewHandler(userCreator port.UserCreator, userQueryer port.UserQueryer, userEditor port.UserEditor, userActivator port.UserActivator, userPasswordChanger port.PasswordChanger) *Handler {
	return &Handler{
		userCreator:         userCreator,
		userQueryer:         userQueryer,
		userEditor:          userEditor,
		userActivator:       userActivator,
		userPasswordChanger: userPasswordChanger,
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
	userResponse, err := h.userCreator.CreateUser(c.Request.Context(), req)
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

	userResponse, err := h.userQueryer.GetUser(c.Request.Context(), req)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, userResponse)
}

// UpdateUser 更新用户
// PUT /api/v1/users/:id
func (h *Handler) UpdateUser(c *gin.Context) {
	var req port.UserBasicInfoRequest
	if err := h.BindJSON(c, &req); err != nil {
		h.ErrorResponse(c, err)
		return
	}
	if ok, err := govalidator.ValidateStruct(req); !ok {
		h.ErrorResponse(c, err)
		return
	}

	userResponse, err := h.userEditor.UpdateBasicInfo(c.Request.Context(), req)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, userResponse)
}
