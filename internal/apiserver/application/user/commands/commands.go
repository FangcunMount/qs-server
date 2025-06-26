package commands

import (
	"context"
	"strings"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/user/dto"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/ports/storage"
	internalErrors "github.com/yshujie/questionnaire-scale/internal/pkg/errors"
)

// CreateUserCommand 创建用户命令
type CreateUserCommand struct {
	Username string `json:"username" binding:"required,min=3,max=50" validate:"required,min=3,max=50"`
	Email    string `json:"email" binding:"required,email" validate:"required,email"`
	Password string `json:"password" binding:"required,min=6" validate:"required,min=6"`
}

// Validate 验证命令
func (cmd CreateUserCommand) Validate() error {
	if strings.TrimSpace(cmd.Username) == "" {
		return internalErrors.NewWithCode(internalErrors.ErrUserInvalidUsername, "用户名不能为空")
	}
	if len(cmd.Username) < 3 || len(cmd.Username) > 50 {
		return internalErrors.NewWithCode(internalErrors.ErrUserInvalidUsername, "用户名长度必须在3-50个字符之间")
	}
	if strings.TrimSpace(cmd.Email) == "" {
		return internalErrors.NewWithCode(internalErrors.ErrUserInvalidEmail, "邮箱不能为空")
	}
	if strings.TrimSpace(cmd.Password) == "" {
		return internalErrors.NewWithCode(internalErrors.ErrUserInvalidPassword, "密码不能为空")
	}
	if len(cmd.Password) < 6 {
		return internalErrors.NewWithCode(internalErrors.ErrUserPasswordTooWeak, "密码长度至少6个字符")
	}
	return nil
}

// UpdateUserCommand 更新用户命令
type UpdateUserCommand struct {
	ID       string  `json:"id" binding:"required"`
	Username *string `json:"username,omitempty" binding:"omitempty,min=3,max=50"`
	Email    *string `json:"email,omitempty" binding:"omitempty,email"`
}

// Validate 验证命令
func (cmd UpdateUserCommand) Validate() error {
	if strings.TrimSpace(cmd.ID) == "" {
		return internalErrors.NewWithCode(internalErrors.ErrUserInvalidID, "用户ID不能为空")
	}
	if cmd.Username != nil && (len(*cmd.Username) < 3 || len(*cmd.Username) > 50) {
		return internalErrors.NewWithCode(internalErrors.ErrUserInvalidUsername, "用户名长度必须在3-50个字符之间")
	}
	return nil
}

// ChangePasswordCommand 修改密码命令
type ChangePasswordCommand struct {
	ID          string `json:"id" binding:"required"`
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=6"`
}

// Validate 验证命令
func (cmd ChangePasswordCommand) Validate() error {
	if strings.TrimSpace(cmd.ID) == "" {
		return internalErrors.NewWithCode(internalErrors.ErrUserInvalidID, "用户ID不能为空")
	}
	if strings.TrimSpace(cmd.OldPassword) == "" {
		return internalErrors.NewWithCode(internalErrors.ErrUserInvalidPassword, "旧密码不能为空")
	}
	if strings.TrimSpace(cmd.NewPassword) == "" {
		return internalErrors.NewWithCode(internalErrors.ErrUserInvalidPassword, "新密码不能为空")
	}
	if len(cmd.NewPassword) < 6 {
		return internalErrors.NewWithCode(internalErrors.ErrUserPasswordTooWeak, "新密码长度至少6个字符")
	}
	return nil
}

// BlockUserCommand 封禁用户命令
type BlockUserCommand struct {
	ID     string `json:"id" binding:"required"`
	Reason string `json:"reason,omitempty"`
}

// Validate 验证命令
func (cmd BlockUserCommand) Validate() error {
	if strings.TrimSpace(cmd.ID) == "" {
		return internalErrors.NewWithCode(internalErrors.ErrUserInvalidID, "用户ID不能为空")
	}
	return nil
}

// ActivateUserCommand 激活用户命令
type ActivateUserCommand struct {
	ID string `json:"id" binding:"required"`
}

// Validate 验证命令
func (cmd ActivateUserCommand) Validate() error {
	if strings.TrimSpace(cmd.ID) == "" {
		return internalErrors.NewWithCode(internalErrors.ErrUserInvalidID, "用户ID不能为空")
	}
	return nil
}

// DeleteUserCommand 删除用户命令
type DeleteUserCommand struct {
	ID string `json:"id" binding:"required"`
}

// Validate 验证命令
func (cmd DeleteUserCommand) Validate() error {
	if strings.TrimSpace(cmd.ID) == "" {
		return internalErrors.NewWithCode(internalErrors.ErrUserInvalidID, "用户ID不能为空")
	}
	return nil
}

// CreateUserHandler 创建用户命令处理器
type CreateUserHandler struct {
	userRepo storage.UserRepository
}

// NewCreateUserHandler 创建命令处理器
func NewCreateUserHandler(userRepo storage.UserRepository) *CreateUserHandler {
	return &CreateUserHandler{userRepo: userRepo}
}

// Handle 处理创建用户命令
func (h *CreateUserHandler) Handle(ctx context.Context, cmd CreateUserCommand) (*dto.UserDTO, error) {
	// 1. 验证命令
	if err := cmd.Validate(); err != nil {
		return nil, err
	}

	// 2. 验证业务规则
	exists, err := h.userRepo.ExistsByUsername(ctx, cmd.Username)
	if err != nil {
		return nil, internalErrors.WrapWithCode(err, internalErrors.ErrUserQueryFailed, "检查用户名是否存在失败")
	}
	if exists {
		return nil, internalErrors.NewWithCode(internalErrors.ErrUsernameAlreadyExists, "用户名已存在")
	}

	exists, err = h.userRepo.ExistsByEmail(ctx, cmd.Email)
	if err != nil {
		return nil, internalErrors.WrapWithCode(err, internalErrors.ErrUserQueryFailed, "检查邮箱是否存在失败")
	}
	if exists {
		return nil, internalErrors.NewWithCode(internalErrors.ErrEmailAlreadyExists, "邮箱已存在")
	}

	// 3. 创建领域对象
	// TODO: 密码应该在这里加密
	u := user.NewUser(cmd.Username, cmd.Email, cmd.Password)

	// 4. 持久化
	if err := h.userRepo.Save(ctx, u); err != nil {
		return nil, internalErrors.WrapWithCode(err, internalErrors.ErrUserCreateFailed, "保存用户失败")
	}

	// 5. 转换为DTO返回
	result := &dto.UserDTO{}
	result.FromDomain(u)
	return result, nil
}

// UpdateUserHandler 更新用户命令处理器
type UpdateUserHandler struct {
	userRepo storage.UserRepository
}

// NewUpdateUserHandler 创建命令处理器
func NewUpdateUserHandler(userRepo storage.UserRepository) *UpdateUserHandler {
	return &UpdateUserHandler{userRepo: userRepo}
}

// Handle 处理更新用户命令
func (h *UpdateUserHandler) Handle(ctx context.Context, cmd UpdateUserCommand) (*dto.UserDTO, error) {
	// 1. 验证命令
	if err := cmd.Validate(); err != nil {
		return nil, err
	}

	// 2. 获取现有用户
	existingUser, err := h.userRepo.FindByID(ctx, user.NewUserID(cmd.ID))
	if err != nil {
		if err == user.ErrUserNotFound {
			return nil, internalErrors.NewWithCode(internalErrors.ErrUserNotFound, "用户不存在")
		}
		return nil, internalErrors.WrapWithCode(err, internalErrors.ErrUserQueryFailed, "查询用户失败")
	}

	// 3. 检查用户名是否被其他用户使用
	if cmd.Username != nil && *cmd.Username != existingUser.Username() {
		exists, err := h.userRepo.ExistsByUsername(ctx, *cmd.Username)
		if err != nil {
			return nil, internalErrors.WrapWithCode(err, internalErrors.ErrUserQueryFailed, "检查用户名是否存在失败")
		}
		if exists {
			return nil, internalErrors.NewWithCode(internalErrors.ErrUsernameAlreadyExists, "用户名已存在")
		}
	}

	// 4. 检查邮箱是否被其他用户使用
	if cmd.Email != nil && *cmd.Email != existingUser.Email() {
		exists, err := h.userRepo.ExistsByEmail(ctx, *cmd.Email)
		if err != nil {
			return nil, internalErrors.WrapWithCode(err, internalErrors.ErrUserQueryFailed, "检查邮箱是否存在失败")
		}
		if exists {
			return nil, internalErrors.NewWithCode(internalErrors.ErrEmailAlreadyExists, "邮箱已存在")
		}
	}

	// 5. 更新用户信息
	if cmd.Username != nil {
		existingUser.ChangeUsername(*cmd.Username)
	}
	if cmd.Email != nil {
		existingUser.ChangeEmail(*cmd.Email)
	}

	// 6. 持久化
	if err := h.userRepo.Update(ctx, existingUser); err != nil {
		return nil, internalErrors.WrapWithCode(err, internalErrors.ErrUserUpdateFailed, "更新用户失败")
	}

	// 7. 转换为DTO返回
	result := &dto.UserDTO{}
	result.FromDomain(existingUser)
	return result, nil
}

// ChangePasswordHandler 修改密码命令处理器
type ChangePasswordHandler struct {
	userRepo storage.UserRepository
}

// NewChangePasswordHandler 创建命令处理器
func NewChangePasswordHandler(userRepo storage.UserRepository) *ChangePasswordHandler {
	return &ChangePasswordHandler{userRepo: userRepo}
}

// Handle 处理修改密码命令
func (h *ChangePasswordHandler) Handle(ctx context.Context, cmd ChangePasswordCommand) error {
	// 1. 验证命令
	if err := cmd.Validate(); err != nil {
		return err
	}

	// 2. 获取现有用户
	existingUser, err := h.userRepo.FindByID(ctx, user.NewUserID(cmd.ID))
	if err != nil {
		if err == user.ErrUserNotFound {
			return internalErrors.NewWithCode(internalErrors.ErrUserNotFound, "用户不存在")
		}
		return internalErrors.WrapWithCode(err, internalErrors.ErrUserQueryFailed, "查询用户失败")
	}

	// 3. 验证旧密码
	// TODO: 实现密码验证逻辑
	if !existingUser.ValidatePassword(cmd.OldPassword) {
		return internalErrors.NewWithCode(internalErrors.ErrUserInvalidPassword, "旧密码不正确")
	}

	// 4. 修改密码
	existingUser.ChangePassword(cmd.NewPassword)

	// 5. 持久化
	if err := h.userRepo.Update(ctx, existingUser); err != nil {
		return internalErrors.WrapWithCode(err, internalErrors.ErrUserPasswordChangeFailed, "修改密码失败")
	}

	return nil
}

// BlockUserHandler 封禁用户命令处理器
type BlockUserHandler struct {
	userRepo storage.UserRepository
}

// NewBlockUserHandler 创建命令处理器
func NewBlockUserHandler(userRepo storage.UserRepository) *BlockUserHandler {
	return &BlockUserHandler{userRepo: userRepo}
}

// Handle 处理封禁用户命令
func (h *BlockUserHandler) Handle(ctx context.Context, cmd BlockUserCommand) error {
	// 1. 验证命令
	if err := cmd.Validate(); err != nil {
		return err
	}

	// 2. 获取现有用户
	existingUser, err := h.userRepo.FindByID(ctx, user.NewUserID(cmd.ID))
	if err != nil {
		if err == user.ErrUserNotFound {
			return internalErrors.NewWithCode(internalErrors.ErrUserNotFound, "用户不存在")
		}
		return internalErrors.WrapWithCode(err, internalErrors.ErrUserQueryFailed, "查询用户失败")
	}

	// 3. 检查用户状态
	if existingUser.IsBlocked() {
		return internalErrors.NewWithCode(internalErrors.ErrUserBlocked, "用户已被封禁")
	}

	// 4. 封禁用户
	existingUser.Block()

	// 5. 持久化
	if err := h.userRepo.Update(ctx, existingUser); err != nil {
		return internalErrors.WrapWithCode(err, internalErrors.ErrUserBlockingFailed, "封禁用户失败")
	}

	return nil
}

// ActivateUserHandler 激活用户命令处理器
type ActivateUserHandler struct {
	userRepo storage.UserRepository
}

// NewActivateUserHandler 创建命令处理器
func NewActivateUserHandler(userRepo storage.UserRepository) *ActivateUserHandler {
	return &ActivateUserHandler{userRepo: userRepo}
}

// Handle 处理激活用户命令
func (h *ActivateUserHandler) Handle(ctx context.Context, cmd ActivateUserCommand) error {
	// 1. 验证命令
	if err := cmd.Validate(); err != nil {
		return err
	}

	// 2. 获取现有用户
	existingUser, err := h.userRepo.FindByID(ctx, user.NewUserID(cmd.ID))
	if err != nil {
		if err == user.ErrUserNotFound {
			return internalErrors.NewWithCode(internalErrors.ErrUserNotFound, "用户不存在")
		}
		return internalErrors.WrapWithCode(err, internalErrors.ErrUserQueryFailed, "查询用户失败")
	}

	// 3. 检查用户状态
	if existingUser.IsActive() {
		return internalErrors.NewWithCode(internalErrors.ErrUserInvalidStatus, "用户已激活")
	}

	// 4. 激活用户
	existingUser.Activate()

	// 5. 持久化
	if err := h.userRepo.Update(ctx, existingUser); err != nil {
		return internalErrors.WrapWithCode(err, internalErrors.ErrUserActivationFailed, "激活用户失败")
	}

	return nil
}

// DeleteUserHandler 删除用户命令处理器
type DeleteUserHandler struct {
	userRepo storage.UserRepository
}

// NewDeleteUserHandler 创建命令处理器
func NewDeleteUserHandler(userRepo storage.UserRepository) *DeleteUserHandler {
	return &DeleteUserHandler{userRepo: userRepo}
}

// Handle 处理删除用户命令
func (h *DeleteUserHandler) Handle(ctx context.Context, cmd DeleteUserCommand) error {
	// 1. 验证命令
	if err := cmd.Validate(); err != nil {
		return err
	}

	// 2. 检查是否存在
	_, err := h.userRepo.FindByID(ctx, user.NewUserID(cmd.ID))
	if err != nil {
		if err == user.ErrUserNotFound {
			return internalErrors.NewWithCode(internalErrors.ErrUserNotFound, "用户不存在")
		}
		return internalErrors.WrapWithCode(err, internalErrors.ErrUserQueryFailed, "查询用户失败")
	}

	// 3. 删除
	if err := h.userRepo.Remove(ctx, user.NewUserID(cmd.ID)); err != nil {
		return internalErrors.WrapWithCode(err, internalErrors.ErrUserDeleteFailed, "删除用户失败")
	}

	return nil
}

// CommandHandlers 命令处理器集合
type CommandHandlers struct {
	CreateUser     *CreateUserHandler
	UpdateUser     *UpdateUserHandler
	ChangePassword *ChangePasswordHandler
	BlockUser      *BlockUserHandler
	ActivateUser   *ActivateUserHandler
	DeleteUser     *DeleteUserHandler
}

// NewCommandHandlers 创建命令处理器集合
func NewCommandHandlers(userRepo storage.UserRepository) *CommandHandlers {
	return &CommandHandlers{
		CreateUser:     NewCreateUserHandler(userRepo),
		UpdateUser:     NewUpdateUserHandler(userRepo),
		ChangePassword: NewChangePasswordHandler(userRepo),
		BlockUser:      NewBlockUserHandler(userRepo),
		ActivateUser:   NewActivateUserHandler(userRepo),
		DeleteUser:     NewDeleteUserHandler(userRepo),
	}
}
