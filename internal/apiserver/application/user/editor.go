package user

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/ports/storage"
	internalErrors "github.com/yshujie/questionnaire-scale/internal/pkg/errors"
)

// UserEditor 用户编辑器 - 负责所有用户相关的写操作
// 面向业务场景，隐藏 CQRS 的技术细节
type UserEditor struct {
	userRepo storage.UserRepository
}

// NewUserEditor 创建用户编辑器
func NewUserEditor(userRepo storage.UserRepository) *UserEditor {
	return &UserEditor{
		userRepo: userRepo,
	}
}

// 用户注册相关业务

// RegisterUser 注册新用户
// 业务场景：用户注册账号
func (e *UserEditor) RegisterUser(ctx context.Context, username, email, password string) (*UserDTO, error) {
	// 验证参数
	if err := e.validateUserRegistration(username, email, password); err != nil {
		return nil, err
	}

	// 检查用户名是否已存在
	exists, err := e.userRepo.ExistsByUsername(ctx, username)
	if err != nil {
		return nil, internalErrors.WrapWithCode(err, internalErrors.ErrUserQueryFailed, "检查用户名是否存在失败")
	}
	if exists {
		return nil, internalErrors.NewWithCode(internalErrors.ErrUsernameAlreadyExists, "用户名已存在")
	}

	// 检查邮箱是否已存在
	exists, err = e.userRepo.ExistsByEmail(ctx, email)
	if err != nil {
		return nil, internalErrors.WrapWithCode(err, internalErrors.ErrUserQueryFailed, "检查邮箱是否存在失败")
	}
	if exists {
		return nil, internalErrors.NewWithCode(internalErrors.ErrEmailAlreadyExists, "邮箱已存在")
	}

	// 创建用户
	newUser := user.NewUser(username, email, password)

	// 保存用户
	if err := e.userRepo.Save(ctx, newUser); err != nil {
		return nil, internalErrors.WrapWithCode(err, internalErrors.ErrUserCreateFailed, "注册用户失败")
	}

	// 返回结果
	result := &UserDTO{}
	result.FromDomain(newUser)
	return result, nil
}

// 用户资料管理相关业务

// UpdateUserProfile 更新用户资料
// 业务场景：用户修改个人信息
func (e *UserEditor) UpdateUserProfile(ctx context.Context, userID string, username *string, email *string) (*UserDTO, error) {
	// 验证参数
	if err := e.validateUserID(userID); err != nil {
		return nil, err
	}

	// 获取现有用户
	existingUser, err := e.getUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	// 检查用户名是否被其他用户使用
	if username != nil && *username != existingUser.Username() {
		if err := e.checkUsernameAvailability(ctx, *username); err != nil {
			return nil, err
		}
		existingUser.ChangeUsername(*username)
	}

	// 检查邮箱是否被其他用户使用
	if email != nil && *email != existingUser.Email() {
		if err := e.checkEmailAvailability(ctx, *email); err != nil {
			return nil, err
		}
		existingUser.ChangeEmail(*email)
	}

	// 保存更新
	if err := e.userRepo.Update(ctx, existingUser); err != nil {
		return nil, internalErrors.WrapWithCode(err, internalErrors.ErrUserUpdateFailed, "更新用户资料失败")
	}

	// 返回结果
	result := &UserDTO{}
	result.FromDomain(existingUser)
	return result, nil
}

// ChangeUserPassword 修改用户密码
// 业务场景：用户修改密码
func (e *UserEditor) ChangeUserPassword(ctx context.Context, userID, oldPassword, newPassword string) error {
	// 验证参数
	if err := e.validatePasswordChange(userID, oldPassword, newPassword); err != nil {
		return err
	}

	// 获取现有用户
	existingUser, err := e.getUserByID(ctx, userID)
	if err != nil {
		return err
	}

	// 验证旧密码
	if !existingUser.ValidatePassword(oldPassword) {
		return internalErrors.NewWithCode(internalErrors.ErrUserInvalidPassword, "旧密码不正确")
	}

	// 修改密码
	existingUser.ChangePassword(newPassword)

	// 保存更新
	if err := e.userRepo.Update(ctx, existingUser); err != nil {
		return internalErrors.WrapWithCode(err, internalErrors.ErrUserPasswordChangeFailed, "修改密码失败")
	}

	return nil
}

// 用户状态管理相关业务

// ActivateUser 激活用户
// 业务场景：管理员激活用户账号
func (e *UserEditor) ActivateUser(ctx context.Context, userID string) error {
	// 验证参数
	if err := e.validateUserID(userID); err != nil {
		return err
	}

	// 获取现有用户
	existingUser, err := e.getUserByID(ctx, userID)
	if err != nil {
		return err
	}

	// 检查用户状态
	if existingUser.IsActive() {
		return internalErrors.NewWithCode(internalErrors.ErrUserInvalidStatus, "用户已经是激活状态")
	}

	// 激活用户
	existingUser.Activate()

	// 保存更新
	if err := e.userRepo.Update(ctx, existingUser); err != nil {
		return internalErrors.WrapWithCode(err, internalErrors.ErrUserActivationFailed, "激活用户失败")
	}

	return nil
}

// BlockUser 封禁用户
// 业务场景：管理员封禁用户账号
func (e *UserEditor) BlockUser(ctx context.Context, userID string, reason string) error {
	// 验证参数
	if err := e.validateUserID(userID); err != nil {
		return err
	}

	// 获取现有用户
	existingUser, err := e.getUserByID(ctx, userID)
	if err != nil {
		return err
	}

	// 检查用户状态
	if existingUser.IsBlocked() {
		return internalErrors.NewWithCode(internalErrors.ErrUserBlocked, "用户已被封禁")
	}

	// 封禁用户
	existingUser.Block()

	// 保存更新
	if err := e.userRepo.Update(ctx, existingUser); err != nil {
		return internalErrors.WrapWithCode(err, internalErrors.ErrUserBlockingFailed, "封禁用户失败")
	}

	return nil
}

// DeleteUser 删除用户
// 业务场景：删除用户账号（软删除或硬删除）
func (e *UserEditor) DeleteUser(ctx context.Context, userID string) error {
	// 验证参数
	if err := e.validateUserID(userID); err != nil {
		return err
	}

	// 检查用户是否存在
	_, err := e.getUserByID(ctx, userID)
	if err != nil {
		return err
	}

	// 删除用户
	if err := e.userRepo.Remove(ctx, user.NewUserID(userID)); err != nil {
		return internalErrors.WrapWithCode(err, internalErrors.ErrUserDeleteFailed, "删除用户失败")
	}

	return nil
}

// 辅助方法

func (e *UserEditor) validateUserRegistration(username, email, password string) error {
	if len(username) < 3 || len(username) > 50 {
		return internalErrors.NewWithCode(internalErrors.ErrUserInvalidUsername, "用户名长度必须在3-50个字符之间")
	}
	if email == "" {
		return internalErrors.NewWithCode(internalErrors.ErrUserInvalidEmail, "邮箱不能为空")
	}
	if len(password) < 6 {
		return internalErrors.NewWithCode(internalErrors.ErrUserPasswordTooWeak, "密码长度至少6个字符")
	}
	return nil
}

func (e *UserEditor) validateUserID(userID string) error {
	if userID == "" {
		return internalErrors.NewWithCode(internalErrors.ErrUserInvalidID, "用户ID不能为空")
	}
	return nil
}

func (e *UserEditor) validatePasswordChange(userID, oldPassword, newPassword string) error {
	if err := e.validateUserID(userID); err != nil {
		return err
	}
	if oldPassword == "" {
		return internalErrors.NewWithCode(internalErrors.ErrUserInvalidPassword, "旧密码不能为空")
	}
	if len(newPassword) < 6 {
		return internalErrors.NewWithCode(internalErrors.ErrUserPasswordTooWeak, "新密码长度至少6个字符")
	}
	return nil
}

func (e *UserEditor) getUserByID(ctx context.Context, userID string) (*user.User, error) {
	existingUser, err := e.userRepo.FindByID(ctx, user.NewUserID(userID))
	if err != nil {
		if err == user.ErrUserNotFound {
			return nil, internalErrors.NewWithCode(internalErrors.ErrUserNotFound, "用户不存在")
		}
		return nil, internalErrors.WrapWithCode(err, internalErrors.ErrUserQueryFailed, "查询用户失败")
	}
	return existingUser, nil
}

func (e *UserEditor) checkUsernameAvailability(ctx context.Context, username string) error {
	exists, err := e.userRepo.ExistsByUsername(ctx, username)
	if err != nil {
		return internalErrors.WrapWithCode(err, internalErrors.ErrUserQueryFailed, "检查用户名是否存在失败")
	}
	if exists {
		return internalErrors.NewWithCode(internalErrors.ErrUsernameAlreadyExists, "用户名已存在")
	}
	return nil
}

func (e *UserEditor) checkEmailAvailability(ctx context.Context, email string) error {
	exists, err := e.userRepo.ExistsByEmail(ctx, email)
	if err != nil {
		return internalErrors.WrapWithCode(err, internalErrors.ErrUserQueryFailed, "检查邮箱是否存在失败")
	}
	if exists {
		return internalErrors.NewWithCode(internalErrors.ErrEmailAlreadyExists, "邮箱已存在")
	}
	return nil
}
