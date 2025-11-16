package user

import "github.com/FangcunMount/qs-server/internal/pkg/meta"

// UserID 用户ID类型
type UserID = meta.ID

// NewUserID 创建用户ID
func NewUserID(id uint64) UserID {
	return meta.FromUint64(id)
}

// Status 用户状态
type Status uint8

const (
	StatusInit     Status = 0 // 初始状态
	StatusActive   Status = 1 // 活跃
	StatusInactive Status = 2 // 非活跃
	StatusBlocked  Status = 3 // 被封禁
)

// Value 获取状态值
func (s Status) Value() uint8 {
	return uint8(s)
}

// String 获取状态字符串
func (s Status) String() string {
	switch s {
	case StatusInit:
		return "init"
	case StatusActive:
		return "active"
	case StatusInactive:
		return "inactive"
	case StatusBlocked:
		return "blocked"
	default:
		return "unknown"
	}
}
