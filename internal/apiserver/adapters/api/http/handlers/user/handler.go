package user

import (
	"github.com/gin-gonic/gin"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/adapters/api/http/handlers"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/user"
)

// Handler 用户HTTP处理器
type Handler struct {
	*handlers.BaseHandler
	userEditor *user.UserEditor
	userQuery  *user.UserQuery
}

// NewHandler 创建用户处理器
func NewHandler(userEditor *user.UserEditor, userQuery *user.UserQuery) handlers.Handler {
	return &Handler{
		BaseHandler: handlers.NewBaseHandler(),
		userEditor:  userEditor,
		userQuery:   userQuery,
	}
}

// GetName 获取Handler名称
func (h *Handler) GetName() string {
	return "user"
}

// CreateUserRequest 创建用户请求
type CreateUserRequest struct {
	Username string `json:"username" binding:"required,min=3,max=50"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
}

// CreateUser 创建用户
// @Summary 创建用户
// @Description 创建新用户
// @Tags users
// @Accept json
// @Produce json
// @Param user body CreateUserRequest true "用户信息"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/users [post]
func (h *Handler) CreateUser(c *gin.Context) {
	var req CreateUserRequest
	if err := h.BindJSON(c, &req); err != nil {
		return
	}

	user, err := h.userEditor.RegisterUser(c.Request.Context(), req.Username, req.Email, req.Password)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponseWithMessage(c, "创建成功", h.userToResponse(user))
}

// GetUser 获取用户详情
// @Summary 获取用户详情
// @Description 根据ID获取用户详情
// @Tags users
// @Accept json
// @Produce json
// @Param id path string true "用户ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/users/{id} [get]
func (h *Handler) GetUser(c *gin.Context) {
	id := h.GetPathParam(c, "id")

	user, err := h.userQuery.GetUserByID(c.Request.Context(), id)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponseWithMessage(c, "获取成功", h.userToResponse(user))
}

// ListUsersRequest 列表查询请求
type ListUsersRequest struct {
	Page     int    `form:"page,default=1"`
	PageSize int    `form:"page_size,default=20"`
	Status   string `form:"status,default=all"`
	Keyword  string `form:"keyword"`
	SortBy   string `form:"sort_by,default=created_at"`
	SortDir  string `form:"sort_dir,default=desc"`
}

// ListUsers 获取用户列表
// @Summary 获取用户列表
// @Description 分页获取用户列表
// @Tags users
// @Accept json
// @Produce json
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Param status query string false "状态筛选"
// @Param keyword query string false "搜索关键字"
// @Param sort_by query string false "排序字段"
// @Param sort_dir query string false "排序方向" Enums(asc, desc)
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/users [get]
func (h *Handler) ListUsers(c *gin.Context) {
	var req ListUsersRequest
	if err := h.BindQuery(c, &req); err != nil {
		return
	}

	query := user.UserListQuery{
		Page:     req.Page,
		PageSize: req.PageSize,
		Status:   req.Status,
		Keyword:  req.Keyword,
		SortBy:   req.SortBy,
		SortDir:  req.SortDir,
	}

	result, err := h.userQuery.GetUserList(c.Request.Context(), query)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	// 转换为响应格式
	items := make([]map[string]interface{}, len(result.Users))
	for i, userDTO := range result.Users {
		items[i] = h.userToResponse(userDTO)
	}

	data := gin.H{
		"items": items,
		"pagination": gin.H{
			"total":       result.Total,
			"page":        result.Page,
			"page_size":   result.PageSize,
			"total_pages": result.TotalPages,
		},
	}

	h.SuccessResponseWithMessage(c, "获取成功", data)
}

// UpdateUserRequest 更新用户请求
type UpdateUserRequest struct {
	Username *string `json:"username,omitempty" binding:"omitempty,min=3,max=50"`
	Email    *string `json:"email,omitempty" binding:"omitempty,email"`
}

// UpdateUser 更新用户
// @Summary 更新用户
// @Description 更新用户信息
// @Tags users
// @Accept json
// @Produce json
// @Param id path string true "用户ID"
// @Param user body UpdateUserRequest true "用户更新信息"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/users/{id} [put]
func (h *Handler) UpdateUser(c *gin.Context) {
	var req UpdateUserRequest
	if err := h.BindJSON(c, &req); err != nil {
		return
	}

	id := h.GetPathParam(c, "id")

	user, err := h.userEditor.UpdateUserProfile(c.Request.Context(), id, req.Username, req.Email)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponseWithMessage(c, "更新成功", h.userToResponse(user))
}

// DeleteUser 删除用户
// @Summary 删除用户
// @Description 删除指定用户
// @Tags users
// @Accept json
// @Produce json
// @Param id path string true "用户ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/users/{id} [delete]
func (h *Handler) DeleteUser(c *gin.Context) {
	id := h.GetPathParam(c, "id")

	err := h.userEditor.DeleteUser(c.Request.Context(), id)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponseWithMessage(c, "删除成功", nil)
}

// ActivateUser 激活用户
// @Summary 激活用户
// @Description 激活指定用户
// @Tags users
// @Accept json
// @Produce json
// @Param id path string true "用户ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/users/{id}/activate [post]
func (h *Handler) ActivateUser(c *gin.Context) {
	id := h.GetPathParam(c, "id")

	err := h.userEditor.ActivateUser(c.Request.Context(), id)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponseWithMessage(c, "激活成功", nil)
}

// BlockUserRequest 封禁用户请求
type BlockUserRequest struct {
	Reason string `json:"reason"`
}

// BlockUser 封禁用户
// @Summary 封禁用户
// @Description 封禁指定用户
// @Tags users
// @Accept json
// @Produce json
// @Param id path string true "用户ID"
// @Param request body BlockUserRequest false "封禁原因"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/users/{id}/block [post]
func (h *Handler) BlockUser(c *gin.Context) {
	var req BlockUserRequest
	_ = h.BindJSON(c, &req) // 忽略错误，因为封禁原因是可选的

	id := h.GetPathParam(c, "id")

	err := h.userEditor.BlockUser(c.Request.Context(), id, req.Reason)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponseWithMessage(c, "封禁成功", nil)
}

// ChangePasswordRequest 修改密码请求
type ChangePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=6"`
}

// ChangePassword 修改密码
// @Summary 修改用户密码
// @Description 修改指定用户的密码
// @Tags users
// @Accept json
// @Produce json
// @Param id path string true "用户ID"
// @Param password body ChangePasswordRequest true "密码信息"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/users/{id}/password [put]
func (h *Handler) ChangePassword(c *gin.Context) {
	var req ChangePasswordRequest
	if err := h.BindJSON(c, &req); err != nil {
		return
	}

	id := h.GetPathParam(c, "id")

	err := h.userEditor.ChangeUserPassword(c.Request.Context(), id, req.OldPassword, req.NewPassword)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponseWithMessage(c, "密码修改成功", nil)
}

// GetUserStats 获取用户统计信息
// @Summary 获取用户统计信息
// @Description 获取用户的统计数据
// @Tags users
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/users/stats [get]
func (h *Handler) GetUserStats(c *gin.Context) {
	stats, err := h.userQuery.GetUserStats(c.Request.Context())
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponseWithMessage(c, "获取成功", stats)
}

// ValidateCredentialsRequest 验证凭证请求
type ValidateCredentialsRequest struct {
	UsernameOrEmail string `json:"username_or_email" binding:"required"`
	Password        string `json:"password" binding:"required"`
}

// ValidateCredentials 验证用户凭证
// @Summary 验证用户凭证
// @Description 验证用户登录凭证
// @Tags users
// @Accept json
// @Produce json
// @Param credentials body ValidateCredentialsRequest true "登录凭证"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /api/v1/users/validate [post]
func (h *Handler) ValidateCredentials(c *gin.Context) {
	var req ValidateCredentialsRequest
	if err := h.BindJSON(c, &req); err != nil {
		return
	}

	user, err := h.userQuery.ValidateUserCredentials(c.Request.Context(), req.UsernameOrEmail, req.Password)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponseWithMessage(c, "验证成功", h.userToResponse(user))
}

// CheckUsernameRequest 检查用户名请求
type CheckUsernameRequest struct {
	Username string `form:"username" binding:"required"`
}

// CheckUsername 检查用户名是否存在
// @Summary 检查用户名是否存在
// @Description 检查用户名的可用性
// @Tags users
// @Accept json
// @Produce json
// @Param username query string true "用户名"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /api/v1/users/check-username [get]
func (h *Handler) CheckUsername(c *gin.Context) {
	var req CheckUsernameRequest
	if err := h.BindQuery(c, &req); err != nil {
		return
	}

	exists, err := h.userQuery.CheckUsernameExists(c.Request.Context(), req.Username)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponseWithMessage(c, "检查完成", gin.H{
		"username":  req.Username,
		"exists":    exists,
		"available": !exists,
	})
}

// CheckEmailRequest 检查邮箱请求
type CheckEmailRequest struct {
	Email string `form:"email" binding:"required,email"`
}

// CheckEmail 检查邮箱是否存在
// @Summary 检查邮箱是否存在
// @Description 检查邮箱的可用性
// @Tags users
// @Accept json
// @Produce json
// @Param email query string true "邮箱"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /api/v1/users/check-email [get]
func (h *Handler) CheckEmail(c *gin.Context) {
	var req CheckEmailRequest
	if err := h.BindQuery(c, &req); err != nil {
		return
	}

	exists, err := h.userQuery.CheckEmailExists(c.Request.Context(), req.Email)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponseWithMessage(c, "检查完成", gin.H{
		"email":     req.Email,
		"exists":    exists,
		"available": !exists,
	})
}

// userToResponse 将DTO转换为响应格式
func (h *Handler) userToResponse(userDTO *user.UserDTO) map[string]interface{} {
	return map[string]interface{}{
		"id":         userDTO.ID,
		"username":   userDTO.Username,
		"email":      userDTO.Email,
		"status":     userDTO.Status,
		"created_at": userDTO.CreatedAt,
		"updated_at": userDTO.UpdatedAt,
	}
}
