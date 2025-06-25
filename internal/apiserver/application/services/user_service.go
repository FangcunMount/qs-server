package services

import (
	"context"
	"fmt"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/ports/storage"
)

// UserService 用户应用服务
type UserService struct {
	userRepo storage.UserRepository
}

// NewUserService 创建用户应用服务
func NewUserService(userRepo storage.UserRepository) *UserService {
	return &UserService{
		userRepo: userRepo,
	}
}

// CreateUserCommand 创建用户命令
type CreateUserCommand struct {
	Username string `json:"username" binding:"required"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
}

// CreateUser 创建用户用例
func (s *UserService) CreateUser(ctx context.Context, cmd CreateUserCommand) (*user.User, error) {
	// 1. 验证用户名是否已存在
	exists, err := s.userRepo.ExistsByUsername(ctx, cmd.Username)
	if err != nil {
		return nil, fmt.Errorf("failed to check username existence: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("username already exists")
	}

	// 2. 验证邮箱是否已存在
	exists, err = s.userRepo.ExistsByEmail(ctx, cmd.Email)
	if err != nil {
		return nil, fmt.Errorf("failed to check email existence: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("email already exists")
	}

	// 3. 创建领域对象
	// TODO: 密码应该在这里加密
	u := user.NewUser(cmd.Username, cmd.Email, cmd.Password)

	// 4. 持久化
	if err := s.userRepo.Save(ctx, u); err != nil {
		return nil, fmt.Errorf("failed to save user: %w", err)
	}

	return u, nil
}

// GetUserQuery 获取用户查询
type GetUserQuery struct {
	ID       *string `form:"id"`
	Username *string `form:"username"`
	Email    *string `form:"email"`
}

// GetUser 获取用户用例
func (s *UserService) GetUser(ctx context.Context, query GetUserQuery) (*user.User, error) {
	if query.ID != nil {
		return s.userRepo.FindByID(ctx, user.NewUserID(*query.ID))
	}

	if query.Username != nil {
		return s.userRepo.FindByUsername(ctx, *query.Username)
	}

	if query.Email != nil {
		return s.userRepo.FindByEmail(ctx, *query.Email)
	}

	return nil, fmt.Errorf("either ID, Username, or Email must be provided")
}

// ListUsersQuery 用户列表查询
type ListUsersQuery struct {
	Page      int          `form:"page"`
	PageSize  int          `form:"page_size"`
	Status    *user.Status `form:"status"`
	Keyword   *string      `form:"keyword"`
	SortBy    string       `form:"sort_by"`
	SortOrder string       `form:"sort_order"`
}

// ListUsersResult 用户列表结果
type ListUsersResult struct {
	Items      []*user.User `json:"items"`
	TotalCount int64        `json:"total_count"`
	HasMore    bool         `json:"has_more"`
	Page       int          `json:"page"`
	PageSize   int          `json:"page_size"`
}

// ListUsers 获取用户列表用例
func (s *UserService) ListUsers(ctx context.Context, query ListUsersQuery) (*ListUsersResult, error) {
	// 设置默认值
	if query.Page <= 0 {
		query.Page = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = 20
	}

	// 构建存储查询
	storageQuery := storage.UserQueryOptions{
		Offset:    (query.Page - 1) * query.PageSize,
		Limit:     query.PageSize,
		Status:    query.Status,
		Keyword:   query.Keyword,
		SortBy:    query.SortBy,
		SortOrder: query.SortOrder,
	}

	// 执行查询
	result, err := s.userRepo.FindUsers(ctx, storageQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to find users: %w", err)
	}

	return &ListUsersResult{
		Items:      result.Items,
		TotalCount: result.TotalCount,
		HasMore:    result.HasMore,
		Page:       query.Page,
		PageSize:   query.PageSize,
	}, nil
}
