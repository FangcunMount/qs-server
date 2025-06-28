package user

import (
	"context"
	"time"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user/port"
	"github.com/yshujie/questionnaire-scale/internal/pkg/code"
	"github.com/yshujie/questionnaire-scale/pkg/auth"
	"github.com/yshujie/questionnaire-scale/pkg/errors"
)

// AuthenticateRequest 认证请求
type AuthenticateRequest struct {
	Username string `json:"username" valid:"required"`
	Password string `json:"password" valid:"required"`
}

// AuthenticateResponse 认证响应
type AuthenticateResponse struct {
	User      *port.UserResponse `json:"user"`
	Token     string             `json:"token,omitempty"`
	ExpiresAt *time.Time         `json:"expires_at,omitempty"`
}

// ValidateTokenRequest 验证令牌请求
type ValidateTokenRequest struct {
	Token string `json:"token" valid:"required"`
}

// AuthService 认证服务
// 负责用户认证、令牌生成和验证等安全相关操作
type AuthService struct {
	userRepo        port.UserRepository
	passwordChanger port.PasswordChanger
	userQueryer     port.UserQueryer
	userActivator   port.UserActivator
}

// NewAuthService 创建认证服务
func NewAuthService(
	userRepo port.UserRepository,
	passwordChanger port.PasswordChanger,
	userQueryer port.UserQueryer,
	userActivator port.UserActivator,
) *AuthService {
	return &AuthService{
		userRepo:        userRepo,
		passwordChanger: passwordChanger,
		userQueryer:     userQueryer,
		userActivator:   userActivator,
	}
}

// Authenticate 用户认证
// 验证用户名和密码，返回用户信息
func (a *AuthService) Authenticate(ctx context.Context, req AuthenticateRequest) (*AuthenticateResponse, error) {
	// 1. 根据用户名查找用户
	userEntity, err := a.userRepo.FindByUsername(ctx, req.Username)
	if err != nil {
		return nil, errors.WithCode(code.ErrUserNotFound, "user not found: %s", req.Username)
	}

	// 2. 检查用户状态
	if !userEntity.IsActive() {
		if userEntity.IsBlocked() {
			return nil, errors.WithCode(code.ErrUserBlocked, "user is blocked")
		}
		if userEntity.IsInactive() {
			return nil, errors.WithCode(code.ErrUserInactive, "user is inactive")
		}
	}

	// 3. 验证密码
	if !a.validatePassword(userEntity.Password(), req.Password) {
		return nil, errors.WithCode(code.ErrPasswordIncorrect, "invalid password")
	}

	// 4. 更新最后登录时间（可选）
	if err := a.updateLastLoginTime(ctx, userEntity); err != nil {
		// 记录错误但不影响认证流程
		// log.Warnw("Failed to update last login time", "user_id", userEntity.ID(), "error", err)
	}

	// 5. 构造用户响应
	userResponse := &port.UserResponse{
		ID:           userEntity.ID().Value(),
		Username:     userEntity.Username(),
		Nickname:     userEntity.Nickname(),
		Email:        userEntity.Email(),
		Phone:        userEntity.Phone(),
		Avatar:       userEntity.Avatar(),
		Introduction: userEntity.Introduction(),
		Status:       userEntity.Status().String(),
		CreatedAt:    userEntity.CreatedAt().Format(time.RFC3339),
		UpdatedAt:    userEntity.UpdatedAt().Format(time.RFC3339),
	}

	return &AuthenticateResponse{
		User: userResponse,
	}, nil
}

// ValidatePasswordOnly 仅验证密码
// 用于Basic认证等场景
func (a *AuthService) ValidatePasswordOnly(ctx context.Context, username, password string) (*port.UserResponse, error) {
	// 使用已有的密码验证服务
	return a.passwordChanger.ValidatePassword(ctx, username, password)
}

// GetUserByUsername 根据用户名获取用户信息
// 用于JWT认证等场景
func (a *AuthService) GetUserByUsername(ctx context.Context, username string) (*port.UserResponse, error) {
	userEntity, err := a.userRepo.FindByUsername(ctx, username)
	if err != nil {
		return nil, errors.WithCode(code.ErrUserNotFound, "user not found: %s", username)
	}

	userResponse := &port.UserResponse{
		ID:           userEntity.ID().Value(),
		Username:     userEntity.Username(),
		Nickname:     userEntity.Nickname(),
		Email:        userEntity.Email(),
		Phone:        userEntity.Phone(),
		Avatar:       userEntity.Avatar(),
		Introduction: userEntity.Introduction(),
		Status:       userEntity.Status().String(),
		CreatedAt:    userEntity.CreatedAt().Format(time.RFC3339),
		UpdatedAt:    userEntity.UpdatedAt().Format(time.RFC3339),
	}

	return userResponse, nil
}

// GetUserByID 根据用户ID获取用户信息
func (a *AuthService) GetUserByID(ctx context.Context, userID uint64) (*port.UserResponse, error) {
	req := port.UserIDRequest{ID: userID}
	return a.userQueryer.GetUser(ctx, req)
}

// GenerateToken 生成JWT令牌
func (a *AuthService) GenerateToken(userResponse *port.UserResponse, secretKey string) (string, time.Time, error) {
	// 设置过期时间（例如24小时）
	expiresAt := time.Now().Add(24 * time.Hour)

	// 使用项目现有的JWT签名方法
	token := auth.Sign(
		userResponse.Username, // secretID
		secretKey,             // secretKey
		"questionnaire-scale", // iss
		userResponse.Username, // aud
	)

	return token, expiresAt, nil
}

// ValidateToken 验证JWT令牌
func (a *AuthService) ValidateToken(ctx context.Context, tokenString string) (*port.UserResponse, error) {
	// TODO: 实现JWT令牌验证逻辑
	// 这里需要解析JWT令牌，验证签名和过期时间
	// 然后根据用户名或用户ID获取用户信息

	// 示例实现：
	// 1. 解析JWT令牌
	// 2. 验证签名和过期时间
	// 3. 从claims中获取用户标识
	// 4. 查询用户信息

	return nil, errors.WithCode(code.ErrTokenInvalid, "token validation not implemented")
}

// IsUserActive 检查用户是否活跃
func (a *AuthService) IsUserActive(ctx context.Context, username string) (bool, error) {
	userEntity, err := a.userRepo.FindByUsername(ctx, username)
	if err != nil {
		return false, err
	}

	return userEntity.IsActive(), nil
}

// ChangePasswordWithAuth 带认证的密码修改
func (a *AuthService) ChangePasswordWithAuth(ctx context.Context, username, oldPassword, newPassword string) error {
	// 1. 首先验证旧密码
	userEntity, err := a.userRepo.FindByUsername(ctx, username)
	if err != nil {
		return errors.WithCode(code.ErrUserNotFound, "user not found")
	}

	if !a.validatePassword(userEntity.Password(), oldPassword) {
		return errors.WithCode(code.ErrPasswordIncorrect, "old password is incorrect")
	}

	// 2. 修改密码
	req := port.UserPasswordChangeRequest{
		ID:          userEntity.ID().Value(),
		OldPassword: oldPassword,
		NewPassword: newPassword,
	}

	return a.passwordChanger.ChangePassword(ctx, req)
}

// validatePassword 验证密码
// 这里应该使用加密后的密码比较
func (a *AuthService) validatePassword(hashedPassword, plainPassword string) bool {
	// 使用项目现有的密码比较方法
	err := auth.Compare(hashedPassword, plainPassword)
	return err == nil
}

// updateLastLoginTime 更新最后登录时间
func (a *AuthService) updateLastLoginTime(ctx context.Context, userEntity *user.User) error {
	// 这里可以更新用户的最后登录时间
	// 可以通过userRepo.Update()方法实现
	// 或者创建一个专门的方法来更新登录时间

	// 示例：
	// userEntity.SetLastLoginTime(time.Now())
	// return a.userRepo.Update(ctx, userEntity)

	return nil // 暂时不实现
}
