package user

import (
	"context"
	"fmt"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user/port"
	"github.com/yshujie/questionnaire-scale/internal/pkg/code"
	"github.com/yshujie/questionnaire-scale/pkg/errors"
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
func (c *UserCreator) CreateUser(ctx context.Context, username, password, nickname, email, phone, introduction string) (*user.User, error) {
	// 唯一性检查
	if c.usernameUnique(ctx, username) {
		return nil, errors.WithCode(code.ErrUserAlreadyExists, "username already exists")
	}
	if c.emailUnique(ctx, email) {
		return nil, errors.WithCode(code.ErrUserAlreadyExists, "email already exists")
	}
	if c.phoneUnique(ctx, phone) {
		return nil, errors.WithCode(code.ErrUserAlreadyExists, "phone already exists")
	}

	// 创建用户领域对象
	userObj := user.NewUserBuilder().
		WithUsername(username).
		WithPassword(password).
		WithNickname(nickname).
		WithEmail(email).
		WithPhone(phone).
		WithStatus(user.StatusInit).
		WithIntroduction(introduction).
		Build()

	// 保存用户
	if err := c.userRepo.Save(ctx, userObj); err != nil {
		return nil, fmt.Errorf("failed to save user: %w", err)
	}

	// 返回用户领域对象
	return userObj, nil
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
