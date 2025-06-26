package user

import (
	"github.com/gin-gonic/gin"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/adapters/api/http/handlers"
	userApp "github.com/yshujie/questionnaire-scale/internal/apiserver/application/user"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/user/commands"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/user/queries"
)

// Handler 用户HTTP处理器
type Handler struct {
	*handlers.BaseHandler
	userService *userApp.Service
}

// NewHandler 创建用户处理器
func NewHandler(userService *userApp.Service) handlers.Handler {
	return &Handler{
		BaseHandler: handlers.NewBaseHandler(),
		userService: userService,
	}
}

// GetName 获取Handler名称
func (h *Handler) GetName() string {
	return "user"
}

// 路由注册已移至 internal/apiserver/routers.go 进行集中管理

// CreateUser 创建用户
// @Summary 创建用户
// @Description 创建新用户
// @Tags users
// @Accept json
// @Produce json
// @Param user body commands.CreateUserCommand true "用户信息"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/users [post]
func (h *Handler) CreateUser(c *gin.Context) {
	var cmd commands.CreateUserCommand
	if err := h.BindJSON(c, &cmd); err != nil {
		return
	}

	user, err := h.userService.CreateUser(c.Request.Context(), cmd)
	if err != nil {
		h.InternalErrorResponse(c, "创建用户失败", err)
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
	query := queries.GetUserQuery{
		ID: &id,
	}

	user, err := h.userService.GetUser(c.Request.Context(), query)
	if err != nil {
		h.NotFoundResponse(c, "用户不存在", err)
		return
	}

	h.SuccessResponseWithMessage(c, "获取成功", h.userToResponse(user))
}

// ListUsers 获取用户列表
// @Summary 获取用户列表
// @Description 分页获取用户列表
// @Tags users
// @Accept json
// @Produce json
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Param status query int false "状态"
// @Param keyword query string false "关键字"
// @Param sort_by query string false "排序字段"
// @Param sort_order query string false "排序方式" Enums(asc, desc)
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/users [get]
func (h *Handler) ListUsers(c *gin.Context) {
	var query queries.ListUsersQuery
	if err := h.BindQuery(c, &query); err != nil {
		return
	}

	result, err := h.userService.ListUsers(c.Request.Context(), query)
	if err != nil {
		h.InternalErrorResponse(c, "获取列表失败", err)
		return
	}

	// 转换为响应格式
	items := make([]map[string]interface{}, len(result.Items))
	for i, user := range result.Items {
		items[i] = h.userToResponse(user)
	}

	data := gin.H{
		"items":      items,
		"pagination": result.Pagination,
	}

	h.SuccessResponseWithMessage(c, "获取成功", data)
}

// UpdateUser 更新用户
// @Summary 更新用户
// @Description 更新用户信息
// @Tags users
// @Accept json
// @Produce json
// @Param id path string true "用户ID"
// @Param user body commands.UpdateUserCommand true "用户更新信息"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/users/{id} [put]
func (h *Handler) UpdateUser(c *gin.Context) {
	var cmd commands.UpdateUserCommand
	if err := h.BindJSON(c, &cmd); err != nil {
		return
	}

	// 从路径参数获取ID
	cmd.ID = h.GetPathParam(c, "id")

	user, err := h.userService.UpdateUser(c.Request.Context(), cmd)
	if err != nil {
		h.InternalErrorResponse(c, "更新用户失败", err)
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
	cmd := commands.DeleteUserCommand{
		ID: h.GetPathParam(c, "id"),
	}

	err := h.userService.DeleteUser(c.Request.Context(), cmd)
	if err != nil {
		h.InternalErrorResponse(c, "删除用户失败", err)
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
	cmd := commands.ActivateUserCommand{
		ID: h.GetPathParam(c, "id"),
	}

	err := h.userService.ActivateUser(c.Request.Context(), cmd)
	if err != nil {
		h.InternalErrorResponse(c, "激活用户失败", err)
		return
	}

	h.SuccessResponseWithMessage(c, "激活成功", nil)
}

// BlockUser 封禁用户
// @Summary 封禁用户
// @Description 封禁指定用户
// @Tags users
// @Accept json
// @Produce json
// @Param id path string true "用户ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/users/{id}/block [post]
func (h *Handler) BlockUser(c *gin.Context) {
	cmd := commands.BlockUserCommand{
		ID: h.GetPathParam(c, "id"),
	}

	err := h.userService.BlockUser(c.Request.Context(), cmd)
	if err != nil {
		h.InternalErrorResponse(c, "封禁用户失败", err)
		return
	}

	h.SuccessResponseWithMessage(c, "封禁成功", nil)
}

// ChangePassword 修改密码
// @Summary 修改用户密码
// @Description 修改指定用户的密码
// @Tags users
// @Accept json
// @Produce json
// @Param id path string true "用户ID"
// @Param password body commands.ChangePasswordCommand true "密码信息"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/users/{id}/password [put]
func (h *Handler) ChangePassword(c *gin.Context) {
	var cmd commands.ChangePasswordCommand
	if err := h.BindJSON(c, &cmd); err != nil {
		return
	}

	// 从路径参数获取ID
	cmd.ID = h.GetPathParam(c, "id")

	err := h.userService.ChangePassword(c.Request.Context(), cmd)
	if err != nil {
		h.InternalErrorResponse(c, "修改密码失败", err)
		return
	}

	h.SuccessResponseWithMessage(c, "密码修改成功", nil)
}

// GetActiveUsers 获取活跃用户
// @Summary 获取活跃用户列表
// @Description 获取所有活跃状态的用户
// @Tags users
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/users/active [get]
func (h *Handler) GetActiveUsers(c *gin.Context) {
	query := queries.GetActiveUsersQuery{}

	users, err := h.userService.GetActiveUsers(c.Request.Context(), query)
	if err != nil {
		h.InternalErrorResponse(c, "获取活跃用户失败", err)
		return
	}

	// 转换为响应格式
	items := make([]map[string]interface{}, len(users))
	for i, user := range users {
		items[i] = h.userToResponse(user)
	}

	h.SuccessResponseWithMessage(c, "获取成功", gin.H{"items": items})
}

// 辅助方法：将DTO转换为响应格式
func (h *Handler) userToResponse(u interface{}) map[string]interface{} {
	// TODO: 实现具体的转换逻辑
	// 这里需要将DTO转换为适合HTTP响应的格式
	return map[string]interface{}{
		"message": "user response conversion not implemented yet",
		"data":    u,
	}
}
