package port

import (
	"context"

	"github.com/fangcun-mount/qs-server/internal/apiserver/domain/user"
)

// UserCreator 用户创建接口
type UserCreator interface {
	CreateUser(ctx context.Context, username, password, nickname, email, phone, introduction string) (*user.User, error)
}

type UserQueryer interface {
	GetUser(ctx context.Context, id uint64) (*user.User, error)
	GetUserByUsername(ctx context.Context, username string) (*user.User, error)
	ListUsers(ctx context.Context, page, pageSize int) ([]*user.User, int64, error)
}

// UserEditor 用户编辑接口
type UserEditor interface {
	UpdateBasicInfo(ctx context.Context, id uint64, username, nickname, email, phone, avatar, introduction string) (*user.User, error)
	UpdateAvatar(ctx context.Context, id uint64, avatar string) error
}

// PasswordChanger 密码管理接口
type PasswordChanger interface {
	ChangePassword(ctx context.Context, id uint64, oldPassword, newPassword string) error
}

// UserActivator 用户状态管理接口
type UserActivator interface {
	ActivateUser(ctx context.Context, id uint64) error
	BlockUser(ctx context.Context, id uint64) error
	DeactivateUser(ctx context.Context, id uint64) error
}
