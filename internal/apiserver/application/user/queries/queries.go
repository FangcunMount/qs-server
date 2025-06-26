package queries

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/shared/interfaces"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/user/dto"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/ports/storage"
	internalErrors "github.com/yshujie/questionnaire-scale/internal/pkg/errors"
)

// GetUserQuery 获取用户查询
type GetUserQuery struct {
	ID       *string `form:"id" json:"id"`
	Username *string `form:"username" json:"username"`
	Email    *string `form:"email" json:"email"`
}

// Validate 验证查询
func (q GetUserQuery) Validate() error {
	if q.ID == nil && q.Username == nil && q.Email == nil {
		return internalErrors.NewWithCode(internalErrors.ErrUserValidationFailed, "必须提供用户ID、用户名或邮箱中的一个")
	}
	return nil
}

// ListUsersQuery 获取用户列表查询
type ListUsersQuery struct {
	interfaces.PaginationRequest
	dto.UserFilterDTO
}

// Validate 验证查询
func (q ListUsersQuery) Validate() error {
	q.PaginationRequest.SetDefaults()
	return nil
}

// SearchUsersQuery 搜索用户查询
type SearchUsersQuery struct {
	interfaces.PaginationRequest
	interfaces.FilterRequest
	interfaces.SortingRequest
}

// Validate 验证查询
func (q SearchUsersQuery) Validate() error {
	q.PaginationRequest.SetDefaults()
	return nil
}

// GetActiveUsersQuery 获取活跃用户查询
type GetActiveUsersQuery struct{}

// Validate 验证查询
func (q GetActiveUsersQuery) Validate() error {
	return nil
}

// GetUserHandler 获取用户查询处理器
type GetUserHandler struct {
	userRepo storage.UserRepository
}

// NewGetUserHandler 创建查询处理器
func NewGetUserHandler(userRepo storage.UserRepository) *GetUserHandler {
	return &GetUserHandler{userRepo: userRepo}
}

// Handle 处理获取用户查询
func (h *GetUserHandler) Handle(ctx context.Context, query GetUserQuery) (*dto.UserDTO, error) {
	// 1. 验证查询
	if err := query.Validate(); err != nil {
		return nil, err
	}

	var foundUser *user.User
	var err error

	// 2. 根据不同条件查找用户
	if query.ID != nil {
		foundUser, err = h.userRepo.FindByID(ctx, user.NewUserID(*query.ID))
	} else if query.Username != nil {
		foundUser, err = h.userRepo.FindByUsername(ctx, *query.Username)
	} else if query.Email != nil {
		foundUser, err = h.userRepo.FindByEmail(ctx, *query.Email)
	}

	if err != nil {
		if err == user.ErrUserNotFound {
			return nil, internalErrors.NewWithCode(internalErrors.ErrUserNotFound, "用户不存在")
		}
		return nil, internalErrors.WrapWithCode(err, internalErrors.ErrUserQueryFailed, "查询用户失败")
	}

	// 3. 转换为DTO
	result := &dto.UserDTO{}
	result.FromDomain(foundUser)
	return result, nil
}

// ListUsersHandler 获取用户列表查询处理器
type ListUsersHandler struct {
	userRepo storage.UserRepository
}

// NewListUsersHandler 创建查询处理器
func NewListUsersHandler(userRepo storage.UserRepository) *ListUsersHandler {
	return &ListUsersHandler{userRepo: userRepo}
}

// Handle 处理获取用户列表查询
func (h *ListUsersHandler) Handle(ctx context.Context, query ListUsersQuery) (*dto.UserListDTO, error) {
	// 1. 验证查询
	if err := query.Validate(); err != nil {
		return nil, err
	}

	// 2. 构建查询选项
	queryOptions := storage.UserQueryOptions{
		Offset:    query.GetOffset(),
		Limit:     query.PageSize,
		Status:    query.Status,
		Keyword:   query.Keyword,
		SortBy:    query.SortBy,
		SortOrder: query.SortOrder,
	}

	// 3. 查询用户列表
	result, err := h.userRepo.FindUsers(ctx, queryOptions)
	if err != nil {
		return nil, internalErrors.WrapWithCode(err, internalErrors.ErrUserQueryFailed, "查询用户列表失败")
	}

	// 4. 转换为DTO
	items := dto.FromDomainList(result.Items)
	pagination := &dto.PaginationResponse{
		Page:       query.Page,
		PageSize:   query.PageSize,
		TotalCount: result.TotalCount,
		HasMore:    result.HasMore,
	}

	return &dto.UserListDTO{
		Items:      items,
		Pagination: pagination,
	}, nil
}

// SearchUsersHandler 搜索用户查询处理器
type SearchUsersHandler struct {
	userRepo storage.UserRepository
}

// NewSearchUsersHandler 创建查询处理器
func NewSearchUsersHandler(userRepo storage.UserRepository) *SearchUsersHandler {
	return &SearchUsersHandler{userRepo: userRepo}
}

// Handle 处理搜索用户查询
func (h *SearchUsersHandler) Handle(ctx context.Context, query SearchUsersQuery) (*dto.UserListDTO, error) {
	// 1. 验证查询
	if err := query.Validate(); err != nil {
		return nil, err
	}

	// 2. 构建搜索查询选项
	queryOptions := storage.UserQueryOptions{
		Offset:    query.GetOffset(),
		Limit:     query.PageSize,
		Keyword:   query.Keyword,
		SortBy:    query.SortBy,
		SortOrder: query.SortOrder,
	}

	// 3. 执行搜索
	result, err := h.userRepo.FindUsers(ctx, queryOptions)
	if err != nil {
		return nil, internalErrors.WrapWithCode(err, internalErrors.ErrUserQueryFailed, "搜索用户失败")
	}

	// 4. 转换为DTO
	items := dto.FromDomainList(result.Items)
	pagination := &dto.PaginationResponse{
		Page:       query.Page,
		PageSize:   query.PageSize,
		TotalCount: result.TotalCount,
		HasMore:    result.HasMore,
	}

	return &dto.UserListDTO{
		Items:      items,
		Pagination: pagination,
	}, nil
}

// GetActiveUsersHandler 获取活跃用户查询处理器
type GetActiveUsersHandler struct {
	userRepo storage.UserRepository
}

// NewGetActiveUsersHandler 创建查询处理器
func NewGetActiveUsersHandler(userRepo storage.UserRepository) *GetActiveUsersHandler {
	return &GetActiveUsersHandler{userRepo: userRepo}
}

// Handle 处理获取活跃用户查询
func (h *GetActiveUsersHandler) Handle(ctx context.Context, query GetActiveUsersQuery) ([]*dto.UserDTO, error) {
	// 1. 验证查询
	if err := query.Validate(); err != nil {
		return nil, err
	}

	// 2. 查询活跃用户
	users, err := h.userRepo.FindActiveUsers(ctx)
	if err != nil {
		return nil, internalErrors.WrapWithCode(err, internalErrors.ErrUserQueryFailed, "查询活跃用户失败")
	}

	// 3. 转换为DTO
	return dto.FromDomainList(users), nil
}

// QueryHandlers 查询处理器集合
type QueryHandlers struct {
	GetUser        *GetUserHandler
	ListUsers      *ListUsersHandler
	SearchUsers    *SearchUsersHandler
	GetActiveUsers *GetActiveUsersHandler
}

// NewQueryHandlers 创建查询处理器集合
func NewQueryHandlers(userRepo storage.UserRepository) *QueryHandlers {
	return &QueryHandlers{
		GetUser:        NewGetUserHandler(userRepo),
		ListUsers:      NewListUsersHandler(userRepo),
		SearchUsers:    NewSearchUsersHandler(userRepo),
		GetActiveUsers: NewGetActiveUsersHandler(userRepo),
	}
}
