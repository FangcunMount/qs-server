package user

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user/port"
)

type UserQueryer struct {
	userRepo port.UserRepository
}

func NewUserQueryer(userRepo port.UserRepository) port.UserQueryer {
	return &UserQueryer{userRepo: userRepo}
}

// GetUser 获取用户
func (q *UserQueryer) GetUser(ctx context.Context, id uint64) (*user.User, error) {
	return q.userRepo.FindByID(ctx, user.NewUserID(id))
}

// GetUserByUsername 根据用户名获取用户信息
func (q *UserQueryer) GetUserByUsername(ctx context.Context, username string) (*user.User, error) {
	return q.userRepo.FindByUsername(ctx, username)
}

// ListUsers 获取用户列表
func (q *UserQueryer) ListUsers(ctx context.Context, page, pageSize int) ([]*user.User, int64, error) {
	users, err := q.userRepo.FindAll(ctx, page, pageSize)
	if err != nil {
		return nil, 0, err
	}
	// TODO: 需要添加总数统计逻辑
	return users, int64(len(users)), nil
}
