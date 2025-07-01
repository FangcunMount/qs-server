package auth

import (
	"context"
	"time"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user/port"
	"github.com/yshujie/questionnaire-scale/internal/pkg/code"
	"github.com/yshujie/questionnaire-scale/pkg/auth"
	"github.com/yshujie/questionnaire-scale/pkg/errors"
)

// Authenticator 认证器
type Authenticator struct {
	userRepo port.UserRepository
}

// NewAuthenticator 创建认证器
func NewAuthenticator(userRepo port.UserRepository) *Authenticator {
	return &Authenticator{
		userRepo: userRepo,
	}
}

// Authenticate 用户认证
// 验证用户名和密码，返回用户信息
func (a *Authenticator) Authenticate(ctx context.Context, req port.AuthenticateRequest) (*port.AuthenticateResponse, error) {
	// 1. 根据用户名查找用户
	user, err := a.userRepo.FindByUsername(ctx, req.Username)
	if err != nil {
		return nil, errors.WithCode(code.ErrUserNotFound, "user not found: %s", req.Username)
	}

	// 2. 检查用户状态
	if !user.IsActive() {
		if user.IsBlocked() {
			return nil, errors.WithCode(code.ErrUserBlocked, "user is blocked")
		}
		if user.IsInactive() {
			return nil, errors.WithCode(code.ErrUserInactive, "user is inactive")
		}
	}

	// 3. 验证密码
	if !a.validatePassword(user.Password(), req.Password) {
		return nil, errors.WithCode(code.ErrPasswordIncorrect, "invalid password")
	}

	// 4. 构造用户响应
	userResponse := &port.UserResponse{
		ID:           user.ID().Value(),
		Username:     user.Username(),
		Nickname:     user.Nickname(),
		Email:        user.Email(),
		Phone:        user.Phone(),
		Avatar:       user.Avatar(),
		Introduction: user.Introduction(),
		Status:       user.Status().String(),
		CreatedAt:    user.CreatedAt().Format(time.RFC3339),
		UpdatedAt:    user.UpdatedAt().Format(time.RFC3339),
	}

	return &port.AuthenticateResponse{
		User: userResponse,
	}, nil
}

// validatePassword 验证密码
func (a *Authenticator) validatePassword(hashedPassword, plainPassword string) bool {
	// 使用项目现有的密码比较方法
	err := auth.Compare(hashedPassword, plainPassword)
	return err == nil
}
