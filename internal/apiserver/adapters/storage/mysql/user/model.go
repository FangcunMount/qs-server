package user

import (
	"time"
)

// Model MySQL 用户表模型
// 专门用于数据库映射，包含 GORM 标签
type Model struct {
	ID        string    `gorm:"primaryKey" json:"id"`
	Username  string    `gorm:"uniqueIndex" json:"username"`
	Email     string    `gorm:"uniqueIndex" json:"email"`
	Password  string    `json:"-"` // 不返回密码
	Status    int       `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName 指定表名
func (Model) TableName() string {
	return "users"
}
