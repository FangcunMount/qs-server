package port

import (
	"context"
)

// UserCreateRequest 创建用户请求
type UserCreateRequest struct {
	Username string `json:"username" valid:"required"`
	Email    string `json:"email" valid:"required,email"`
	Password string `json:"password" valid:"required"`
}

// UserQueryRequest 查询用户请求
type UserIDRequest struct {
	ID string `json:"id" valid:"required"`
}

// UserUpdateRequest 更新用户请求
type UserUpdateRequest struct {
	ID       string `json:"id" valid:"required"`
	Username string `json:"username" valid:"optional"`
	Email    string `json:"email" valid:"optional"`
}

// UserPasswordChangeRequest 修改密码请求
type UserPasswordChangeRequest struct {
	ID          string `json:"id" valid:"required"`
	OldPassword string `json:"old_password" valid:"required"`
	NewPassword string `json:"new_password" valid:"required"`
}

// UserResponse 用户响应
type UserResponse struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	Email     string `json:"email"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// UserListResponse 用户列表响应
type UserListResponse struct {
	Users      []*UserResponse `json:"users"`
	TotalCount int64           `json:"total_count"`
	Page       int             `json:"page"`
	PageSize   int             `json:"page_size"`
}

// UserService 用户服务接口（入站端口）
// 定义了所有用户相关的业务用例
type UserService interface {
	// 用户管理
	CreateUser(ctx context.Context, req UserCreateRequest) (*UserResponse, error)
	GetUser(ctx context.Context, req UserIDRequest) (*UserResponse, error)
	UpdateUser(ctx context.Context, req UserUpdateRequest) (*UserResponse, error)
	DeleteUser(ctx context.Context, req UserIDRequest) error

	// 用户列表和查询
	ListUsers(ctx context.Context, page, pageSize int) (*UserListResponse, error)

	// 用户状态管理
	ActivateUser(ctx context.Context, req UserIDRequest) error
	BlockUser(ctx context.Context, req UserIDRequest) error
	DeactivateUser(ctx context.Context, req UserIDRequest) error

	// 密码管理
	ChangePassword(ctx context.Context, req UserPasswordChangeRequest) error
	ValidatePassword(ctx context.Context, username, password string) (*UserResponse, error)
}
