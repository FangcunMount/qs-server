package user

import (
	"context"
	"time"

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
func (q *UserQueryer) GetUser(ctx context.Context, req port.UserIDRequest) (*port.UserResponse, error) {
	user, err := q.userRepo.FindByID(ctx, user.NewUserID(req.ID))
	if err != nil {
		return nil, err
	}

	userResponse := &port.UserResponse{
		ID:        user.ID().Value(),
		Username:  user.Username(),
		Nickname:  user.Nickname(),
		Email:     user.Email(),
		Phone:     user.Phone(),
		Avatar:    user.Avatar(),
		Status:    user.Status().String(),
		CreatedAt: user.CreatedAt().Format(time.RFC3339),
		UpdatedAt: user.UpdatedAt().Format(time.RFC3339),
	}

	return userResponse, nil
}

// GetUserByUsername 根据用户名获取用户信息
func (q *UserQueryer) GetUserByUsername(ctx context.Context, username string) (*port.UserResponse, error) {
	user, err := q.userRepo.FindByUsername(ctx, username)
	if err != nil {
		return nil, err
	}

	userResponse := &port.UserResponse{
		ID:        user.ID().Value(),
		Username:  user.Username(),
		Nickname:  user.Nickname(),
		Email:     user.Email(),
		Phone:     user.Phone(),
		Avatar:    user.Avatar(),
		Status:    user.Status().String(),
		CreatedAt: user.CreatedAt().Format(time.RFC3339),
		UpdatedAt: user.UpdatedAt().Format(time.RFC3339),
	}

	return userResponse, nil
}

// ListUsers 获取用户列表
func (q *UserQueryer) ListUsers(ctx context.Context, page, pageSize int) (*port.UserListResponse, error) {
	users, err := q.userRepo.FindAll(ctx, page, pageSize)
	if err != nil {
		return nil, err
	}

	userListResponse := make([]*port.UserResponse, 0, len(users))
	for _, user := range users {
		userListResponse = append(userListResponse, &port.UserResponse{
			ID:       user.ID().Value(),
			Username: user.Username(),
			Nickname: user.Nickname(),
			Email:    user.Email(),
			Phone:    user.Phone(),
			Avatar:   user.Avatar(),
			Status:   user.Status().String(),
		})
	}

	return &port.UserListResponse{
		Users: userListResponse,
	}, nil
}
