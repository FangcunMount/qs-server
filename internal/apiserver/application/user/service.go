package user

import (
	"context"
	"fmt"
	"time"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user/port"
)

// Service 用户应用服务
// 实现用户相关的所有业务用例
type Service struct {
	userRepo port.UserRepository
}

// NewService 创建用户服务
func NewService(userRepo port.UserRepository) port.UserService {
	return &Service{
		userRepo: userRepo,
	}
}

// CreateUser 创建用户
func (s *Service) CreateUser(ctx context.Context, req port.UserCreateRequest) (*port.UserResponse, error) {
	// 验证输入
	if err := s.validateCreateRequest(&req); err != nil {
		return nil, err
	}

	// 检查用户名是否已存在
	exists, err := s.userRepo.ExistsByUsername(ctx, req.Username)
	if err != nil {
		return nil, fmt.Errorf("failed to check username existence: %w", err)
	}
	if exists {
		return nil, user.ErrDuplicateUsername
	}

	// 检查邮箱是否已存在
	exists, err = s.userRepo.ExistsByEmail(ctx, req.Email)
	if err != nil {
		return nil, fmt.Errorf("failed to check email existence: %w", err)
	}
	if exists {
		return nil, user.ErrDuplicateEmail
	}

	// 创建用户领域对象
	newUser := user.NewUser(req.Username, req.Email, req.Password)

	// 保存用户
	if err := s.userRepo.Save(ctx, newUser); err != nil {
		return nil, fmt.Errorf("failed to save user: %w", err)
	}

	// 返回响应
	return s.toUserResponse(newUser), nil
}

// GetUser 获取用户
func (s *Service) GetUser(ctx context.Context, req port.UserIDRequest) (*port.UserResponse, error) {
	userID := user.NewUserID(req.ID)

	userDomain, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	return s.toUserResponse(userDomain), nil
}

// UpdateUser 更新用户
func (s *Service) UpdateUser(ctx context.Context, req port.UserUpdateRequest) (*port.UserResponse, error) {
	userID := user.NewUserID(req.ID)

	// 获取现有用户
	userDomain, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	// 更新用户名
	if req.Username != "" && req.Username != userDomain.Username() {
		// 检查新用户名是否可用
		exists, err := s.userRepo.ExistsByUsername(ctx, req.Username)
		if err != nil {
			return nil, fmt.Errorf("failed to check username existence: %w", err)
		}
		if exists {
			return nil, user.ErrDuplicateUsername
		}

		if err := userDomain.ChangeUsername(req.Username); err != nil {
			return nil, err
		}
	}

	// 更新邮箱
	if req.Email != "" && req.Email != userDomain.Email() {
		// 检查新邮箱是否可用
		exists, err := s.userRepo.ExistsByEmail(ctx, req.Email)
		if err != nil {
			return nil, fmt.Errorf("failed to check email existence: %w", err)
		}
		if exists {
			return nil, user.ErrDuplicateEmail
		}

		if err := userDomain.ChangeEmail(req.Email); err != nil {
			return nil, err
		}
	}

	// 保存更新
	if err := s.userRepo.Update(ctx, userDomain); err != nil {
		return nil, fmt.Errorf("failed to update user: %w", err)
	}

	return s.toUserResponse(userDomain), nil
}

// DeleteUser 删除用户
func (s *Service) DeleteUser(ctx context.Context, req port.UserIDRequest) error {
	userID := user.NewUserID(req.ID)

	// 检查用户是否存在
	_, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return err
	}

	// 删除用户
	return s.userRepo.Remove(ctx, userID)
}

// ListUsers 获取用户列表
func (s *Service) ListUsers(ctx context.Context, page, pageSize int) (*port.UserListResponse, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}

	offset := (page - 1) * pageSize

	// 获取用户列表
	users, err := s.userRepo.FindAll(ctx, pageSize, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to find users: %w", err)
	}

	// 获取总数
	totalCount, err := s.userRepo.Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to count users: %w", err)
	}

	// 转换为响应
	userResponses := make([]*port.UserResponse, 0, len(users))
	for _, userDomain := range users {
		userResponses = append(userResponses, s.toUserResponse(userDomain))
	}

	return &port.UserListResponse{
		Users:      userResponses,
		TotalCount: totalCount,
		Page:       page,
		PageSize:   pageSize,
	}, nil
}

// ActivateUser 激活用户
func (s *Service) ActivateUser(ctx context.Context, req port.UserIDRequest) error {
	userID := user.NewUserID(req.ID)

	userDomain, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return err
	}

	if err := userDomain.Activate(); err != nil {
		return err
	}

	return s.userRepo.Update(ctx, userDomain)
}

// BlockUser 封禁用户
func (s *Service) BlockUser(ctx context.Context, req port.UserIDRequest) error {
	userID := user.NewUserID(req.ID)

	userDomain, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return err
	}

	if err := userDomain.Block(); err != nil {
		return err
	}

	return s.userRepo.Update(ctx, userDomain)
}

// DeactivateUser 停用用户
func (s *Service) DeactivateUser(ctx context.Context, req port.UserIDRequest) error {
	userID := user.NewUserID(req.ID)

	userDomain, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return err
	}

	if err := userDomain.Deactivate(); err != nil {
		return err
	}

	return s.userRepo.Update(ctx, userDomain)
}

// ChangePassword 修改密码
func (s *Service) ChangePassword(ctx context.Context, req port.UserPasswordChangeRequest) error {
	userID := user.NewUserID(req.ID)

	userDomain, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return err
	}

	// 验证旧密码
	if !userDomain.ValidatePassword(req.OldPassword) {
		return user.ErrInvalidPassword
	}

	// 修改密码
	if err := userDomain.ChangePassword(req.NewPassword); err != nil {
		return err
	}

	return s.userRepo.Update(ctx, userDomain)
}

// ValidatePassword 验证密码
func (s *Service) ValidatePassword(ctx context.Context, username, password string) (*port.UserResponse, error) {
	userDomain, err := s.userRepo.FindByUsername(ctx, username)
	if err != nil {
		return nil, err
	}

	if !userDomain.ValidatePassword(password) {
		return nil, user.ErrInvalidPassword
	}

	return s.toUserResponse(userDomain), nil
}

// 辅助方法

// validateCreateRequest 验证创建用户请求
func (s *Service) validateCreateRequest(req *port.UserCreateRequest) error {
	if req.Username == "" {
		return fmt.Errorf("username is required")
	}
	if len(req.Username) < 3 || len(req.Username) > 50 {
		return fmt.Errorf("username must be between 3 and 50 characters")
	}
	if req.Email == "" {
		return fmt.Errorf("email is required")
	}
	if req.Password == "" {
		return fmt.Errorf("password is required")
	}
	if len(req.Password) < 6 {
		return fmt.Errorf("password must be at least 6 characters")
	}
	return nil
}

// toUserResponse 将领域对象转换为响应对象
func (s *Service) toUserResponse(userDomain *user.User) *port.UserResponse {
	return &port.UserResponse{
		ID:        userDomain.ID().Value(),
		Username:  userDomain.Username(),
		Email:     userDomain.Email(),
		Status:    userDomain.Status().String(),
		CreatedAt: userDomain.CreatedAt().Format(time.RFC3339),
		UpdatedAt: userDomain.UpdatedAt().Format(time.RFC3339),
	}
}

// 扩展的方法 - 这些不在接口中，但对其他层有用

// GetUserByUsername 根据用户名获取用户
func (s *Service) GetUserByUsername(ctx context.Context, username string) (*port.UserResponse, error) {
	userDomain, err := s.userRepo.FindByUsername(ctx, username)
	if err != nil {
		return nil, err
	}

	return s.toUserResponse(userDomain), nil
}

// GetUserByEmail 根据邮箱获取用户
func (s *Service) GetUserByEmail(ctx context.Context, email string) (*port.UserResponse, error) {
	userDomain, err := s.userRepo.FindByEmail(ctx, email)
	if err != nil {
		return nil, err
	}

	return s.toUserResponse(userDomain), nil
}

// ChangePasswordWithID 使用用户ID修改密码
func (s *Service) ChangePasswordWithID(ctx context.Context, id string, req port.UserPasswordChangeRequest) error {
	userID := user.NewUserID(id)

	userDomain, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return err
	}

	// 验证旧密码
	if !userDomain.ValidatePassword(req.OldPassword) {
		return user.ErrInvalidPassword
	}

	// 修改密码
	if err := userDomain.ChangePassword(req.NewPassword); err != nil {
		return err
	}

	return s.userRepo.Update(ctx, userDomain)
}

// GetUserStats 获取用户统计
func (s *Service) GetUserStats(ctx context.Context) (map[string]interface{}, error) {
	// 总用户数
	totalCount, err := s.userRepo.Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to count total users: %w", err)
	}

	// 活跃用户数
	activeCount, err := s.userRepo.CountByStatus(ctx, user.StatusActive)
	if err != nil {
		return nil, fmt.Errorf("failed to count active users: %w", err)
	}

	// 非活跃用户数
	inactiveCount, err := s.userRepo.CountByStatus(ctx, user.StatusInactive)
	if err != nil {
		return nil, fmt.Errorf("failed to count inactive users: %w", err)
	}

	// 封禁用户数
	blockedCount, err := s.userRepo.CountByStatus(ctx, user.StatusBlocked)
	if err != nil {
		return nil, fmt.Errorf("failed to count blocked users: %w", err)
	}

	return map[string]interface{}{
		"total_users":    totalCount,
		"active_users":   activeCount,
		"inactive_users": inactiveCount,
		"blocked_users":  blockedCount,
	}, nil
}

// CheckUsername 检查用户名是否可用
func (s *Service) CheckUsername(ctx context.Context, username string) (bool, error) {
	exists, err := s.userRepo.ExistsByUsername(ctx, username)
	if err != nil {
		return false, err
	}
	return !exists, nil // true表示可用
}

// CheckEmail 检查邮箱是否可用
func (s *Service) CheckEmail(ctx context.Context, email string) (bool, error) {
	exists, err := s.userRepo.ExistsByEmail(ctx, email)
	if err != nil {
		return false, err
	}
	return !exists, nil // true表示可用
}
