package queries

import (
	"context"

	appErrors "github.com/yshujie/questionnaire-scale/internal/apiserver/application/shared/errors"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/shared/interfaces"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/user/dto"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/ports/storage"
)

// GetUserQuery 获取用户查询
type GetUserQuery struct {
	ID       *string `form:"id" json:"id"`
	Username *string `form:"username" json:"username"`
	Email    *string `form:"email" json:"email"`
}

// Validate 验证查询
func (q *GetUserQuery) Validate() error {
	if q.ID == nil && q.Username == nil && q.Email == nil {
		return appErrors.NewValidationError("identifier", "ID, Username, or Email must be provided")
	}
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

	var u *user.User
	var err error

	// 2. 执行查询
	if query.ID != nil {
		u, err = h.userRepo.FindByID(ctx, user.NewUserID(*query.ID))
	} else if query.Username != nil {
		u, err = h.userRepo.FindByUsername(ctx, *query.Username)
	} else if query.Email != nil {
		u, err = h.userRepo.FindByEmail(ctx, *query.Email)
	}

	if err != nil {
		if err == user.ErrUserNotFound {
			identifier := ""
			if query.ID != nil {
				identifier = *query.ID
			} else if query.Username != nil {
				identifier = *query.Username
			} else {
				identifier = *query.Email
			}
			return nil, appErrors.NewNotFoundError("user", identifier)
		}
		return nil, appErrors.NewSystemError("Failed to find user", err)
	}

	// 3. 转换为DTO返回
	result := &dto.UserDTO{}
	result.FromDomain(u)
	return result, nil
}

// ListUsersQuery 用户列表查询
type ListUsersQuery struct {
	interfaces.PaginationRequest
	dto.UserFilterDTO
}

// Validate 验证查询
func (q *ListUsersQuery) Validate() error {
	q.PaginationRequest.SetDefaults()
	q.UserFilterDTO.SetDefaults()
	return nil
}

// ListUsersHandler 用户列表查询处理器
type ListUsersHandler struct {
	userRepo storage.UserRepository
}

// NewListUsersHandler 创建查询处理器
func NewListUsersHandler(userRepo storage.UserRepository) *ListUsersHandler {
	return &ListUsersHandler{userRepo: userRepo}
}

// Handle 处理用户列表查询
func (h *ListUsersHandler) Handle(ctx context.Context, query ListUsersQuery) (*dto.UserListDTO, error) {
	// 1. 验证查询
	if err := query.Validate(); err != nil {
		return nil, err
	}

	// 2. 构建存储查询
	storageQuery := storage.UserQueryOptions{
		Offset:    query.GetOffset(),
		Limit:     query.PageSize,
		Status:    query.Status,
		Keyword:   query.Keyword,
		SortBy:    query.SortBy,
		SortOrder: query.SortOrder,
	}

	// 3. 执行查询
	result, err := h.userRepo.FindUsers(ctx, storageQuery)
	if err != nil {
		return nil, appErrors.NewSystemError("Failed to find users", err)
	}

	// 4. 转换为DTO
	userDTOs := dto.FromDomainList(result.Items)
	pagination := &dto.PaginationResponse{
		Page:       query.Page,
		PageSize:   query.PageSize,
		TotalCount: result.TotalCount,
		HasMore:    result.HasMore,
	}

	return &dto.UserListDTO{
		Items:      userDTOs,
		Pagination: pagination,
	}, nil
}

// SearchUsersQuery 用户搜索查询
type SearchUsersQuery struct {
	interfaces.PaginationRequest
	interfaces.FilterRequest
	interfaces.SortingRequest

	AdvancedFilters dto.UserFilterDTO `json:"filters"`
}

// Validate 验证查询
func (q *SearchUsersQuery) Validate() error {
	q.PaginationRequest.SetDefaults()
	q.SortingRequest.SetDefaults("created_at")
	q.AdvancedFilters.SetDefaults()
	return nil
}

// SearchUsersHandler 用户搜索查询处理器
type SearchUsersHandler struct {
	userRepo storage.UserRepository
}

// NewSearchUsersHandler 创建查询处理器
func NewSearchUsersHandler(userRepo storage.UserRepository) *SearchUsersHandler {
	return &SearchUsersHandler{userRepo: userRepo}
}

// Handle 处理用户搜索查询
func (h *SearchUsersHandler) Handle(ctx context.Context, query SearchUsersQuery) (*dto.UserListDTO, error) {
	// 1. 验证查询
	if err := query.Validate(); err != nil {
		return nil, err
	}

	// 2. 构建复杂查询
	storageQuery := storage.UserQueryOptions{
		Offset:    query.GetOffset(),
		Limit:     query.PageSize,
		SortBy:    query.SortBy,
		SortOrder: query.SortOrder,
	}

	// 应用高级过滤器
	if query.AdvancedFilters.HasStatusFilter() {
		status := query.AdvancedFilters.GetStatus()
		storageQuery.Status = &status
	}
	if query.AdvancedFilters.HasKeyword() {
		keyword := query.AdvancedFilters.GetKeyword()
		storageQuery.Keyword = &keyword
	}

	// 应用基础过滤器
	if query.FilterRequest.Keyword != nil {
		storageQuery.Keyword = query.FilterRequest.Keyword
	}

	// 3. 执行查询
	result, err := h.userRepo.FindUsers(ctx, storageQuery)
	if err != nil {
		return nil, appErrors.NewSystemError("Failed to search users", err)
	}

	// 4. 转换为DTO
	userDTOs := dto.FromDomainList(result.Items)
	pagination := &dto.PaginationResponse{
		Page:       query.Page,
		PageSize:   query.PageSize,
		TotalCount: result.TotalCount,
		HasMore:    result.HasMore,
	}

	return &dto.UserListDTO{
		Items:      userDTOs,
		Pagination: pagination,
	}, nil
}

// GetActiveUsersQuery 获取活跃用户查询
type GetActiveUsersQuery struct{}

// Validate 验证查询
func (q *GetActiveUsersQuery) Validate() error {
	return nil
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

	// 2. 执行查询
	users, err := h.userRepo.FindActiveUsers(ctx)
	if err != nil {
		return nil, appErrors.NewSystemError("Failed to find active users", err)
	}

	// 3. 转换为DTO返回
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
