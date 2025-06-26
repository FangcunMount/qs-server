package user

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/shared/interfaces"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/user/commands"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/user/dto"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/user/queries"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/ports/storage"
)

// Service 用户应用服务
// 作为聚合根的统一入口，协调命令和查询处理器
type Service struct {
	// 命令处理器
	commandHandlers *commands.CommandHandlers
	// 查询处理器
	queryHandlers *queries.QueryHandlers
	// 仓储
	userRepo storage.UserRepository
}

// NewService 创建用户应用服务
func NewService(userRepo storage.UserRepository) *Service {
	return &Service{
		commandHandlers: commands.NewCommandHandlers(userRepo),
		queryHandlers:   queries.NewQueryHandlers(userRepo),
		userRepo:        userRepo,
	}
}

// ServiceName 实现ApplicationService接口
func (s *Service) ServiceName() string {
	return "user-service"
}

// ExecuteInTransaction 在事务中执行操作
func (s *Service) ExecuteInTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	// 这里可以实现事务逻辑
	// 简化实现，直接执行函数
	return fn(ctx)
}

// 命令处理方法

// CreateUser 创建用户
func (s *Service) CreateUser(ctx context.Context, cmd commands.CreateUserCommand) (*dto.UserDTO, error) {
	return s.commandHandlers.CreateUser.Handle(ctx, cmd)
}

// UpdateUser 更新用户
func (s *Service) UpdateUser(ctx context.Context, cmd commands.UpdateUserCommand) (*dto.UserDTO, error) {
	return s.commandHandlers.UpdateUser.Handle(ctx, cmd)
}

// ChangePassword 修改密码
func (s *Service) ChangePassword(ctx context.Context, cmd commands.ChangePasswordCommand) error {
	return s.commandHandlers.ChangePassword.Handle(ctx, cmd)
}

// BlockUser 封禁用户
func (s *Service) BlockUser(ctx context.Context, cmd commands.BlockUserCommand) error {
	return s.commandHandlers.BlockUser.Handle(ctx, cmd)
}

// ActivateUser 激活用户
func (s *Service) ActivateUser(ctx context.Context, cmd commands.ActivateUserCommand) error {
	return s.commandHandlers.ActivateUser.Handle(ctx, cmd)
}

// DeleteUser 删除用户
func (s *Service) DeleteUser(ctx context.Context, cmd commands.DeleteUserCommand) error {
	return s.commandHandlers.DeleteUser.Handle(ctx, cmd)
}

// 查询处理方法

// GetUser 获取用户
func (s *Service) GetUser(ctx context.Context, query queries.GetUserQuery) (*dto.UserDTO, error) {
	return s.queryHandlers.GetUser.Handle(ctx, query)
}

// ListUsers 获取用户列表
func (s *Service) ListUsers(ctx context.Context, query queries.ListUsersQuery) (*dto.UserListDTO, error) {
	return s.queryHandlers.ListUsers.Handle(ctx, query)
}

// SearchUsers 搜索用户
func (s *Service) SearchUsers(ctx context.Context, query queries.SearchUsersQuery) (*dto.UserListDTO, error) {
	return s.queryHandlers.SearchUsers.Handle(ctx, query)
}

// GetActiveUsers 获取活跃用户
func (s *Service) GetActiveUsers(ctx context.Context, query queries.GetActiveUsersQuery) ([]*dto.UserDTO, error) {
	return s.queryHandlers.GetActiveUsers.Handle(ctx, query)
}

// 高级用例方法 - 组合多个操作

// CreateAndActivateUser 创建并激活用户
func (s *Service) CreateAndActivateUser(ctx context.Context, createCmd commands.CreateUserCommand) (*dto.UserDTO, error) {
	// 在事务中执行
	var result *dto.UserDTO
	err := s.ExecuteInTransaction(ctx, func(ctx context.Context) error {
		// 1. 创建用户
		user, err := s.CreateUser(ctx, createCmd)
		if err != nil {
			return err
		}

		// 2. 激活用户（如果需要的话）
		activateCmd := commands.ActivateUserCommand{ID: user.ID}
		if err := s.ActivateUser(ctx, activateCmd); err != nil {
			return err
		}

		// 3. 重新获取更新后的用户
		getQuery := queries.GetUserQuery{ID: &user.ID}
		result, err = s.GetUser(ctx, getQuery)
		return err
	})

	return result, err
}

// BulkUpdateUserStatus 批量更新用户状态
func (s *Service) BulkUpdateUserStatus(ctx context.Context, userIDs []string, action string) error {
	return s.ExecuteInTransaction(ctx, func(ctx context.Context) error {
		for _, id := range userIDs {
			switch action {
			case "activate":
				cmd := commands.ActivateUserCommand{ID: id}
				if err := s.ActivateUser(ctx, cmd); err != nil {
					return err
				}
			case "block":
				cmd := commands.BlockUserCommand{ID: id}
				if err := s.BlockUser(ctx, cmd); err != nil {
					return err
				}
			case "delete":
				cmd := commands.DeleteUserCommand{ID: id}
				if err := s.DeleteUser(ctx, cmd); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// GetUsersByStatus 获取指定状态的用户列表
func (s *Service) GetUsersByStatus(ctx context.Context, status int, pagination interfaces.PaginationRequest) (*dto.UserListDTO, error) {
	// 转换状态值
	userStatus := dto.UserFilterDTO{
		Status: (*user.Status)(ptrStatus(status)),
	}

	query := queries.ListUsersQuery{
		PaginationRequest: pagination,
		UserFilterDTO:     userStatus,
	}
	return s.ListUsers(ctx, query)
}

// SearchUsersByKeyword 按关键字搜索用户
func (s *Service) SearchUsersByKeyword(ctx context.Context, keyword string, pagination interfaces.PaginationRequest) (*dto.UserListDTO, error) {
	query := queries.SearchUsersQuery{
		PaginationRequest: pagination,
		FilterRequest: interfaces.FilterRequest{
			Keyword: &keyword,
		},
		SortingRequest: interfaces.SortingRequest{
			SortBy:    "created_at",
			SortOrder: "desc",
		},
	}
	return s.SearchUsers(ctx, query)
}

// ValidateUser 验证用户完整性
func (s *Service) ValidateUser(ctx context.Context, userID string) (map[string]interface{}, error) {
	// 获取用户
	getQuery := queries.GetUserQuery{ID: &userID}
	user, err := s.GetUser(ctx, getQuery)
	if err != nil {
		return nil, err
	}

	// 执行验证逻辑
	validation := map[string]interface{}{
		"user_id":      userID,
		"valid":        true,
		"issues":       []string{},
		"has_username": user.Username != "",
		"has_email":    user.Email != "",
		"is_active":    user.Status == 1, // StatusActive
	}

	var issues []string

	// 检查用户名
	if user.Username == "" {
		issues = append(issues, "Missing username")
		validation["valid"] = false
	}

	// 检查邮箱
	if user.Email == "" {
		issues = append(issues, "Missing email")
		validation["valid"] = false
	}

	// 检查状态
	if user.Status != 1 { // StatusActive
		issues = append(issues, "User is not active")
		validation["valid"] = false
	}

	validation["issues"] = issues
	return validation, nil
}

// 辅助函数
func ptr(i int) *int {
	return &i
}

func ptrStatus(i int) *user.Status {
	status := user.Status(i)
	return &status
}
