package user

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/ports/storage"
	internalErrors "github.com/yshujie/questionnaire-scale/internal/pkg/errors"
)

// UserQuery 用户查询器 - 负责所有用户相关的读操作
// 面向业务场景，提供各种用户查询和搜索功能
type UserQuery struct {
	userRepo storage.UserRepository
}

// NewUserQuery 创建用户查询器
func NewUserQuery(userRepo storage.UserRepository) *UserQuery {
	return &UserQuery{
		userRepo: userRepo,
	}
}

// 单个用户查询相关业务

// GetUserByID 根据ID获取用户
// 业务场景：查看用户详情、用户资料页面
func (q *UserQuery) GetUserByID(ctx context.Context, userID string) (*UserDTO, error) {
	// 验证参数
	if userID == "" {
		return nil, internalErrors.NewWithCode(internalErrors.ErrUserInvalidID, "用户ID不能为空")
	}

	// 查询用户
	existingUser, err := q.userRepo.FindByID(ctx, user.NewUserID(userID))
	if err != nil {
		if err == user.ErrUserNotFound {
			return nil, internalErrors.NewWithCode(internalErrors.ErrUserNotFound, "用户不存在")
		}
		return nil, internalErrors.WrapWithCode(err, internalErrors.ErrUserQueryFailed, "查询用户失败")
	}

	// 转换为DTO
	result := &UserDTO{}
	result.FromDomain(existingUser)
	return result, nil
}

// GetUserByUsername 根据用户名获取用户
// 业务场景：通过用户名查找用户、用户搜索
func (q *UserQuery) GetUserByUsername(ctx context.Context, username string) (*UserDTO, error) {
	// 验证参数
	if username == "" {
		return nil, internalErrors.NewWithCode(internalErrors.ErrUserInvalidUsername, "用户名不能为空")
	}

	// 查询用户
	existingUser, err := q.userRepo.FindByUsername(ctx, username)
	if err != nil {
		if err == user.ErrUserNotFound {
			return nil, internalErrors.NewWithCode(internalErrors.ErrUserNotFound, "用户不存在")
		}
		return nil, internalErrors.WrapWithCode(err, internalErrors.ErrUserQueryFailed, "查询用户失败")
	}

	// 转换为DTO
	result := &UserDTO{}
	result.FromDomain(existingUser)
	return result, nil
}

// GetUserByEmail 根据邮箱获取用户
// 业务场景：邮箱登录、邮箱找回密码
func (q *UserQuery) GetUserByEmail(ctx context.Context, email string) (*UserDTO, error) {
	// 验证参数
	if email == "" {
		return nil, internalErrors.NewWithCode(internalErrors.ErrUserInvalidEmail, "邮箱不能为空")
	}

	// 查询用户
	existingUser, err := q.userRepo.FindByEmail(ctx, email)
	if err != nil {
		if err == user.ErrUserNotFound {
			return nil, internalErrors.NewWithCode(internalErrors.ErrUserNotFound, "用户不存在")
		}
		return nil, internalErrors.WrapWithCode(err, internalErrors.ErrUserQueryFailed, "查询用户失败")
	}

	// 转换为DTO
	result := &UserDTO{}
	result.FromDomain(existingUser)
	return result, nil
}

// 用户列表查询相关业务

// UserListQuery 用户列表查询参数
type UserListQuery struct {
	Page     int    // 页码，从1开始
	PageSize int    // 每页大小
	Status   string // 用户状态筛选：active, inactive, blocked, all
	Keyword  string // 搜索关键词（用户名或邮箱）
	SortBy   string // 排序字段：created_at, username, email
	SortDir  string // 排序方向：asc, desc
}

// UserListResult 用户列表查询结果
type UserListResult struct {
	Users      []*UserDTO `json:"users"`
	Total      int64      `json:"total"`
	Page       int        `json:"page"`
	PageSize   int        `json:"page_size"`
	TotalPages int        `json:"total_pages"`
}

// GetUserList 获取用户列表
// 业务场景：用户管理页面、用户列表展示、用户搜索
func (q *UserQuery) GetUserList(ctx context.Context, query UserListQuery) (*UserListResult, error) {
	// 验证和设置默认值
	if err := q.validateListQuery(&query); err != nil {
		return nil, err
	}

	// 构建存储层查询参数
	var status *user.Status
	if query.Status != "all" {
		switch query.Status {
		case "active":
			activeStatus := user.StatusActive
			status = &activeStatus
		case "inactive":
			inactiveStatus := user.StatusInactive
			status = &inactiveStatus
		case "blocked":
			blockedStatus := user.StatusBlocked
			status = &blockedStatus
		}
	}

	var keyword *string
	if query.Keyword != "" {
		keyword = &query.Keyword
	}

	storageQuery := storage.UserQueryOptions{
		Offset:    (query.Page - 1) * query.PageSize,
		Limit:     query.PageSize,
		Status:    status,
		Keyword:   keyword,
		SortBy:    query.SortBy,
		SortOrder: query.SortDir,
	}

	// 查询用户列表
	result, err := q.userRepo.FindUsers(ctx, storageQuery)
	if err != nil {
		return nil, internalErrors.WrapWithCode(err, internalErrors.ErrUserListQueryFailed, "查询用户列表失败")
	}

	// 转换为DTO
	userDTOs := make([]*UserDTO, 0, len(result.Items))
	for _, u := range result.Items {
		dto := &UserDTO{}
		dto.FromDomain(u)
		userDTOs = append(userDTOs, dto)
	}

	// 计算总页数
	totalPages := int(result.TotalCount) / query.PageSize
	if int(result.TotalCount)%query.PageSize > 0 {
		totalPages++
	}

	return &UserListResult{
		Users:      userDTOs,
		Total:      result.TotalCount,
		Page:       query.Page,
		PageSize:   query.PageSize,
		TotalPages: totalPages,
	}, nil
}

// 用户统计查询相关业务

// UserStats 用户统计信息
type UserStats struct {
	TotalUsers    int64 `json:"total_users"`
	ActiveUsers   int64 `json:"active_users"`
	InactiveUsers int64 `json:"inactive_users"`
	BlockedUsers  int64 `json:"blocked_users"`
}

// GetUserStats 获取用户统计信息
// 业务场景：管理后台数据展示、用户概览统计
func (q *UserQuery) GetUserStats(ctx context.Context) (*UserStats, error) {
	// 简化实现：分别查询各状态的用户数量
	allQuery := storage.UserQueryOptions{Offset: 0, Limit: 1}
	allResult, err := q.userRepo.FindUsers(ctx, allQuery)
	if err != nil {
		return nil, internalErrors.WrapWithCode(err, internalErrors.ErrUserStatsQueryFailed, "查询用户统计信息失败")
	}

	activeStatus := user.StatusActive
	activeQuery := storage.UserQueryOptions{Status: &activeStatus, Offset: 0, Limit: 1}
	activeResult, err := q.userRepo.FindUsers(ctx, activeQuery)
	if err != nil {
		return nil, internalErrors.WrapWithCode(err, internalErrors.ErrUserStatsQueryFailed, "查询用户统计信息失败")
	}

	inactiveStatus := user.StatusInactive
	inactiveQuery := storage.UserQueryOptions{Status: &inactiveStatus, Offset: 0, Limit: 1}
	inactiveResult, err := q.userRepo.FindUsers(ctx, inactiveQuery)
	if err != nil {
		return nil, internalErrors.WrapWithCode(err, internalErrors.ErrUserStatsQueryFailed, "查询用户统计信息失败")
	}

	blockedStatus := user.StatusBlocked
	blockedQuery := storage.UserQueryOptions{Status: &blockedStatus, Offset: 0, Limit: 1}
	blockedResult, err := q.userRepo.FindUsers(ctx, blockedQuery)
	if err != nil {
		return nil, internalErrors.WrapWithCode(err, internalErrors.ErrUserStatsQueryFailed, "查询用户统计信息失败")
	}

	return &UserStats{
		TotalUsers:    allResult.TotalCount,
		ActiveUsers:   activeResult.TotalCount,
		InactiveUsers: inactiveResult.TotalCount,
		BlockedUsers:  blockedResult.TotalCount,
	}, nil
}

// 用户验证查询相关业务

// CheckUsernameExists 检查用户名是否存在
// 业务场景：用户注册时的用户名可用性检查
func (q *UserQuery) CheckUsernameExists(ctx context.Context, username string) (bool, error) {
	// 验证参数
	if username == "" {
		return false, internalErrors.NewWithCode(internalErrors.ErrUserInvalidUsername, "用户名不能为空")
	}

	// 检查用户名是否存在
	exists, err := q.userRepo.ExistsByUsername(ctx, username)
	if err != nil {
		return false, internalErrors.WrapWithCode(err, internalErrors.ErrUserQueryFailed, "检查用户名是否存在失败")
	}

	return exists, nil
}

// CheckEmailExists 检查邮箱是否存在
// 业务场景：用户注册时的邮箱可用性检查
func (q *UserQuery) CheckEmailExists(ctx context.Context, email string) (bool, error) {
	// 验证参数
	if email == "" {
		return false, internalErrors.NewWithCode(internalErrors.ErrUserInvalidEmail, "邮箱不能为空")
	}

	// 检查邮箱是否存在
	exists, err := q.userRepo.ExistsByEmail(ctx, email)
	if err != nil {
		return false, internalErrors.WrapWithCode(err, internalErrors.ErrUserQueryFailed, "检查邮箱是否存在失败")
	}

	return exists, nil
}

// ValidateUserCredentials 验证用户凭证
// 业务场景：用户登录验证
func (q *UserQuery) ValidateUserCredentials(ctx context.Context, usernameOrEmail, password string) (*UserDTO, error) {
	// 验证参数
	if usernameOrEmail == "" {
		return nil, internalErrors.NewWithCode(internalErrors.ErrUserInvalidCredentials, "用户名或邮箱不能为空")
	}
	if password == "" {
		return nil, internalErrors.NewWithCode(internalErrors.ErrUserInvalidCredentials, "密码不能为空")
	}

	// 先尝试用用户名查找
	var existingUser *user.User
	var err error

	existingUser, err = q.userRepo.FindByUsername(ctx, usernameOrEmail)
	if err == user.ErrUserNotFound {
		// 如果用用户名找不到，再尝试用邮箱查找
		existingUser, err = q.userRepo.FindByEmail(ctx, usernameOrEmail)
	}

	if err != nil {
		if err == user.ErrUserNotFound {
			return nil, internalErrors.NewWithCode(internalErrors.ErrUserInvalidCredentials, "用户名或密码错误")
		}
		return nil, internalErrors.WrapWithCode(err, internalErrors.ErrUserQueryFailed, "查询用户失败")
	}

	// 验证密码
	if !existingUser.ValidatePassword(password) {
		return nil, internalErrors.NewWithCode(internalErrors.ErrUserInvalidCredentials, "用户名或密码错误")
	}

	// 检查用户状态
	if existingUser.IsBlocked() {
		return nil, internalErrors.NewWithCode(internalErrors.ErrUserBlocked, "用户已被封禁")
	}

	// 转换为DTO
	result := &UserDTO{}
	result.FromDomain(existingUser)
	return result, nil
}

// 辅助方法

func (q *UserQuery) validateListQuery(query *UserListQuery) error {
	// 设置默认值
	if query.Page <= 0 {
		query.Page = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = 20
	}
	if query.PageSize > 100 {
		query.PageSize = 100 // 限制最大页面大小
	}
	if query.Status == "" {
		query.Status = "all"
	}
	if query.SortBy == "" {
		query.SortBy = "created_at"
	}
	if query.SortDir == "" {
		query.SortDir = "desc"
	}

	// 验证参数
	validStatuses := map[string]bool{
		"all":      true,
		"active":   true,
		"inactive": true,
		"blocked":  true,
	}
	if !validStatuses[query.Status] {
		return internalErrors.NewWithCode(internalErrors.ErrUserInvalidStatus, "无效的用户状态")
	}

	validSortFields := map[string]bool{
		"created_at": true,
		"username":   true,
		"email":      true,
	}
	if !validSortFields[query.SortBy] {
		return internalErrors.NewWithCode(internalErrors.ErrUserInvalidSortField, "无效的排序字段")
	}

	validSortDirs := map[string]bool{
		"asc":  true,
		"desc": true,
	}
	if !validSortDirs[query.SortDir] {
		return internalErrors.NewWithCode(internalErrors.ErrUserInvalidSortDirection, "无效的排序方向")
	}

	return nil
}
