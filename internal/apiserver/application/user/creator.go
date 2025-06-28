package user

import (
	"context"
	"fmt"
	"time"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user/port"
	"github.com/yshujie/questionnaire-scale/internal/pkg/errors"
)

// UserCreator 用户创建器
type UserCreator struct {
	userRepo port.UserRepository
}

// NewUserCreator 创建用户创建器
func NewUserCreator(userRepo port.UserRepository) port.UserCreator {
	return &UserCreator{userRepo: userRepo}
}

// CreateUser 创建用户
func (c *UserCreator) CreateUser(ctx context.Context, req port.UserCreateRequest) (*port.UserResponse, error) {
	// 唯一性检查
	if c.usernameUnique(ctx, req.Username) {
		return nil, errors.NewWithCode(errors.ErrUserAlreadyExists, "username already exists")
	}
	if c.emailUnique(ctx, req.Email) {
		return nil, errors.NewWithCode(errors.ErrUserAlreadyExists, "email already exists")
	}
	if c.phoneUnique(ctx, req.Phone) {
		return nil, errors.NewWithCode(errors.ErrUserAlreadyExists, "phone already exists")
	}

	// 创建用户领域对象
	user := user.NewUserBuilder().
		WithUsername(req.Username).
		WithNickname(req.Nickname).
		WithEmail(req.Email).
		WithPhone(req.Phone).
		WithStatus(user.StatusInit).
		WithIntroduction(req.Introduction).
		Build()

	// 保存用户
	if err := c.userRepo.Save(ctx, user); err != nil {
		return nil, fmt.Errorf("failed to save user: %w", err)
	}

	// 返回响应
	return &port.UserResponse{
		ID:           user.ID().Value(),
		Username:     user.Username(),
		Nickname:     user.Nickname(),
		Avatar:       user.Avatar(),
		Introduction: user.Introduction(),
		Email:        user.Email(),
		Phone:        user.Phone(),
		Status:       user.Status().String(),
		CreatedAt:    user.CreatedAt().Format(time.DateTime),
		UpdatedAt:    user.UpdatedAt().Format(time.DateTime),
	}, nil
}

func (c *UserCreator) usernameUnique(ctx context.Context, username string) bool {
	return c.userRepo.ExistsByUsername(ctx, username)
}

func (c *UserCreator) emailUnique(ctx context.Context, email string) bool {
	return c.userRepo.ExistsByEmail(ctx, email)
}

func (c *UserCreator) phoneUnique(ctx context.Context, phone string) bool {
	return c.userRepo.ExistsByPhone(ctx, phone)
}
