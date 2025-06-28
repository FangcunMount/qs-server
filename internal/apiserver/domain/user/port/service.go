package port

import (
	"context"
)

// UserCreateRequest 创建用户请求
type UserCreateRequest struct {
	Username     string `json:"username" valid:"required"`
	Password     string `json:"password" valid:"required,min=6"`
	Nickname     string `json:"nickname" valid:"required"`
	Email        string `json:"email" valid:"required,email"`
	Phone        string `json:"phone" valid:"required"`
	Introduction string `json:"introduction" valid:"optional"`
}

// UserQueryRequest 查询用户请求
type UserIDRequest struct {
	ID uint64 `json:"id" valid:"required"`
}

// UserUpdateRequest 更新用户请求
type UserBasicInfoRequest struct {
	ID           uint64 `json:"id" valid:"required"`
	Username     string `json:"username" valid:"optional"`
	Nickname     string `json:"nickname" valid:"optional"`
	Email        string `json:"email" valid:"optional"`
	Phone        string `json:"phone" valid:"optional"`
	Avatar       string `json:"avatar" valid:"optional"`
	Introduction string `json:"introduction" valid:"optional"`
}

type UserAvatarRequest struct {
	ID     uint64 `json:"id" valid:"required"`
	Avatar string `json:"avatar" valid:"required"`
}

// UserPasswordChangeRequest 修改密码请求
type UserPasswordChangeRequest struct {
	ID          uint64 `json:"id" valid:"required"`
	OldPassword string `json:"old_password" valid:"required"`
	NewPassword string `json:"new_password" valid:"required"`
}

// UserResponse 用户响应
type UserResponse struct {
	ID           uint64 `json:"id"`
	Username     string `json:"username"`
	Nickname     string `json:"nickname"`
	Phone        string `json:"phone"`
	Avatar       string `json:"avatar"`
	Introduction string `json:"introduction"`
	Email        string `json:"email"`
	Status       string `json:"status"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

// UserListResponse 用户列表响应
type UserListResponse struct {
	Users      []*UserResponse `json:"users"`
	TotalCount int64           `json:"total_count"`
	Page       int             `json:"page"`
	PageSize   int             `json:"page_size"`
}

// UserCreator 用户创建接口
type UserCreator interface {
	CreateUser(ctx context.Context, req UserCreateRequest) (*UserResponse, error)
}

type UserQueryer interface {
	GetUser(ctx context.Context, req UserIDRequest) (*UserResponse, error)
	ListUsers(ctx context.Context, page, pageSize int) (*UserListResponse, error)
}

// UserEditor 用户编辑接口
type UserEditor interface {
	UpdateBasicInfo(ctx context.Context, req UserBasicInfoRequest) (*UserResponse, error)
	UpdateAvatar(ctx context.Context, req UserAvatarRequest) error
}

// PasswordChanger 密码管理接口
type PasswordChanger interface {
	ChangePassword(ctx context.Context, req UserPasswordChangeRequest) error
	ValidatePassword(ctx context.Context, username, password string) (*UserResponse, error)
}

// UserActivator 用户状态管理接口
type UserActivator interface {
	ActivateUser(ctx context.Context, req UserIDRequest) error
	BlockUser(ctx context.Context, req UserIDRequest) error
	DeactivateUser(ctx context.Context, req UserIDRequest) error
}
