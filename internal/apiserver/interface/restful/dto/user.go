package dto

import "time"

// UserCreateRequest 创建用户请求
type UserCreateRequest struct {
	Username     string `json:"username" valid:"required"`
	Password     string `json:"password" valid:"required,stringlength(6|50)"`
	Nickname     string `json:"nickname" valid:"required"`
	Email        string `json:"email" valid:"required,email"`
	Phone        string `json:"phone" valid:"required"`
	Introduction string `json:"introduction" valid:"optional"`
}

// UserIDRequest 用户ID请求
type UserIDRequest struct {
	ID uint64 `json:"id" valid:"required"`
}

// UserBasicInfoRequest 更新用户基本信息请求
type UserBasicInfoRequest struct {
	ID           uint64 `json:"id" valid:"required"`
	Username     string `json:"username" valid:"optional"`
	Nickname     string `json:"nickname" valid:"optional"`
	Email        string `json:"email" valid:"optional"`
	Phone        string `json:"phone" valid:"optional"`
	Avatar       string `json:"avatar" valid:"optional"`
	Introduction string `json:"introduction" valid:"optional"`
}

// UserAvatarRequest 更新用户头像请求
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

// AuthenticateRequest 认证请求
type AuthenticateRequest struct {
	Username string `json:"username" valid:"required"`
	Password string `json:"password" valid:"required"`
}

// AuthenticateResponse 认证响应
type AuthenticateResponse struct {
	User      *UserResponse `json:"user"`
	Token     string        `json:"token,omitempty"`
	ExpiresAt *time.Time    `json:"expires_at,omitempty"`
}
