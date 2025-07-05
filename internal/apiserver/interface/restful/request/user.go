package request

// UserIDRequest 用户ID请求
type UserIDRequest struct {
	ID uint64 `json:"id" valid:"required"`
}
