package user

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user/port"
)

// UserActivator 用户状态管理器
type UserActivator struct {
	userRepo port.UserRepository
}

// NewUserActivator 创建用户状态管理器
func NewUserActivator(userRepo port.UserRepository) port.UserActivator {
	return &UserActivator{userRepo: userRepo}
}

// ActivateUser 激活用户
func (a *UserActivator) ActivateUser(ctx context.Context, req port.UserIDRequest) error {
	user, err := a.userRepo.FindByID(ctx, user.NewUserID(req.ID))
	if err != nil {
		return err
	}

	if err := user.Activate(); err != nil {
		return err
	}

	if err := a.userRepo.Update(ctx, user); err != nil {
		return err
	}

	return nil
}

// BlockUser 封禁用户
func (a *UserActivator) BlockUser(ctx context.Context, req port.UserIDRequest) error {
	user, err := a.userRepo.FindByID(ctx, user.NewUserID(req.ID))
	if err != nil {
		return err
	}

	if err := user.Block(); err != nil {
		return err
	}

	if err := a.userRepo.Update(ctx, user); err != nil {
		return err
	}

	return nil
}

// DeactivateUser 禁用用户
func (a *UserActivator) DeactivateUser(ctx context.Context, req port.UserIDRequest) error {
	user, err := a.userRepo.FindByID(ctx, user.NewUserID(req.ID))
	if err != nil {
		return err
	}

	if err := user.Deactivate(); err != nil {
		return err
	}

	if err := a.userRepo.Update(ctx, user); err != nil {
		return err
	}

	return nil
}
