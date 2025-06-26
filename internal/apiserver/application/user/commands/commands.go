package commands

import (
	"context"

	appErrors "github.com/yshujie/questionnaire-scale/internal/apiserver/application/shared/errors"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/user/dto"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/ports/storage"
)

// CreateUserCommand 创建用户命令
type CreateUserCommand struct {
	Username string `json:"username" binding:"required"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
}

// Validate 验证命令
func (cmd *CreateUserCommand) Validate() error {
	if cmd.Username == "" {
		return appErrors.NewValidationError("username", "Username is required")
	}
	if cmd.Email == "" {
		return appErrors.NewValidationError("email", "Email is required")
	}
	if cmd.Password == "" || len(cmd.Password) < 6 {
		return appErrors.NewValidationError("password", "Password must be at least 6 characters")
	}
	return nil
}

// UpdateUserCommand 更新用户命令
type UpdateUserCommand struct {
	ID       string `json:"id" binding:"required"`
	Username string `json:"username"`
	Email    string `json:"email,omitempty,email"`
}

// Validate 验证命令
func (cmd *UpdateUserCommand) Validate() error {
	if cmd.ID == "" {
		return appErrors.NewValidationError("id", "User ID is required")
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
func (cmd *ChangePasswordCommand) Validate() error {
	if cmd.ID == "" {
		return appErrors.NewValidationError("id", "User ID is required")
	}
	if cmd.NewPassword == "" || len(cmd.NewPassword) < 6 {
		return appErrors.NewValidationError("new_password", "New password must be at least 6 characters")
	}
	return nil
}

// BlockUserCommand 封禁用户命令
type BlockUserCommand struct {
	ID string `json:"id" binding:"required"`
}

// Validate 验证命令
func (cmd *BlockUserCommand) Validate() error {
	if cmd.ID == "" {
		return appErrors.NewValidationError("id", "User ID is required")
	}
	return nil
}

// ActivateUserCommand 激活用户命令
type ActivateUserCommand struct {
	ID string `json:"id" binding:"required"`
}

// Validate 验证命令
func (cmd *ActivateUserCommand) Validate() error {
	if cmd.ID == "" {
		return appErrors.NewValidationError("id", "User ID is required")
	}
	return nil
}

// DeleteUserCommand 删除用户命令
type DeleteUserCommand struct {
	ID string `json:"id" binding:"required"`
}

// Validate 验证命令
func (cmd *DeleteUserCommand) Validate() error {
	if cmd.ID == "" {
		return appErrors.NewValidationError("id", "User ID is required")
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
		return nil, appErrors.NewSystemError("Failed to check username existence", err)
	}
	if exists {
		return nil, appErrors.NewValidationError("username", "Username already exists")
	}

	exists, err = h.userRepo.ExistsByEmail(ctx, cmd.Email)
	if err != nil {
		return nil, appErrors.NewSystemError("Failed to check email existence", err)
	}
	if exists {
		return nil, appErrors.NewValidationError("email", "Email already exists")
	}

	// 3. 创建领域对象
	// TODO: 密码应该在这里加密
	u := user.NewUser(cmd.Username, cmd.Email, cmd.Password)

	// 4. 持久化
	if err := h.userRepo.Save(ctx, u); err != nil {
		return nil, appErrors.NewSystemError("Failed to save user", err)
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

	// 2. 获取领域对象
	u, err := h.userRepo.FindByID(ctx, user.NewUserID(cmd.ID))
	if err != nil {
		if err == user.ErrUserNotFound {
			return nil, appErrors.NewNotFoundError("user", cmd.ID)
		}
		return nil, appErrors.NewSystemError("Failed to find user", err)
	}

	// 3. 执行业务操作
	// TODO: 实现用户更新逻辑，例如更新用户名、邮箱等

	// 4. 持久化
	if err := h.userRepo.Update(ctx, u); err != nil {
		return nil, appErrors.NewSystemError("Failed to update user", err)
	}

	// 5. 转换为DTO返回
	result := &dto.UserDTO{}
	result.FromDomain(u)
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

	// 2. 获取领域对象
	u, err := h.userRepo.FindByID(ctx, user.NewUserID(cmd.ID))
	if err != nil {
		if err == user.ErrUserNotFound {
			return appErrors.NewNotFoundError("user", cmd.ID)
		}
		return appErrors.NewSystemError("Failed to find user", err)
	}

	// 3. 验证旧密码
	// TODO: 实现密码验证逻辑
	if u.Password() != cmd.OldPassword {
		return appErrors.NewValidationError("old_password", "Invalid old password")
	}

	// 4. 执行业务操作
	u.ChangePassword(cmd.NewPassword)

	// 5. 持久化
	if err := h.userRepo.Update(ctx, u); err != nil {
		return appErrors.NewSystemError("Failed to update user password", err)
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

	// 2. 获取领域对象
	u, err := h.userRepo.FindByID(ctx, user.NewUserID(cmd.ID))
	if err != nil {
		if err == user.ErrUserNotFound {
			return appErrors.NewNotFoundError("user", cmd.ID)
		}
		return appErrors.NewSystemError("Failed to find user", err)
	}

	// 3. 执行业务操作
	u.Block()

	// 4. 持久化
	if err := h.userRepo.Update(ctx, u); err != nil {
		return appErrors.NewSystemError("Failed to block user", err)
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

	// 2. 获取领域对象
	u, err := h.userRepo.FindByID(ctx, user.NewUserID(cmd.ID))
	if err != nil {
		if err == user.ErrUserNotFound {
			return appErrors.NewNotFoundError("user", cmd.ID)
		}
		return appErrors.NewSystemError("Failed to find user", err)
	}

	// 3. 执行业务操作
	u.Activate()

	// 4. 持久化
	if err := h.userRepo.Update(ctx, u); err != nil {
		return appErrors.NewSystemError("Failed to activate user", err)
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
			return appErrors.NewNotFoundError("user", cmd.ID)
		}
		return appErrors.NewSystemError("Failed to check user existence", err)
	}

	// 3. 删除
	if err := h.userRepo.Remove(ctx, user.NewUserID(cmd.ID)); err != nil {
		return appErrors.NewSystemError("Failed to delete user", err)
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
