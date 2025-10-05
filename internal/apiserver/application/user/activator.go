package user

import (
	"context"

	"github.com/fangcun-mount/qs-server/internal/apiserver/domain/user"
	"github.com/fangcun-mount/qs-server/internal/apiserver/domain/user/port"
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
func (a *UserActivator) ActivateUser(ctx context.Context, id uint64) error {
	userObj, err := a.userRepo.FindByID(ctx, user.NewUserID(id))
	if err != nil {
		return err
	}

	if err := userObj.Activate(); err != nil {
		return err
	}

	return a.userRepo.Update(ctx, userObj)
}

// BlockUser 封禁用户
func (a *UserActivator) BlockUser(ctx context.Context, id uint64) error {
	userObj, err := a.userRepo.FindByID(ctx, user.NewUserID(id))
	if err != nil {
		return err
	}

	if err := userObj.Block(); err != nil {
		return err
	}

	return a.userRepo.Update(ctx, userObj)
}

// DeactivateUser 禁用用户
func (a *UserActivator) DeactivateUser(ctx context.Context, id uint64) error {
	userObj, err := a.userRepo.FindByID(ctx, user.NewUserID(id))
	if err != nil {
		return err
	}

	if err := userObj.Deactivate(); err != nil {
		return err
	}

	return a.userRepo.Update(ctx, userObj)
}
