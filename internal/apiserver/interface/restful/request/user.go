package request

import "github.com/FangcunMount/qs-server/internal/pkg/meta"

// UserIDRequest 用户ID请求
type UserIDRequest struct {
	ID meta.ID `json:"id" valid:"required"`
}
