package response

import (
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// UserResponse 用户响应
type UserResponse struct {
	ID           meta.ID `json:"id"`
	Username     string  `json:"username"`
	Nickname     string  `json:"nickname"`
	Phone        string  `json:"phone"`
	Avatar       string  `json:"avatar"`
	Introduction string  `json:"introduction"`
	Email        string  `json:"email"`
	Status       string  `json:"status"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
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
