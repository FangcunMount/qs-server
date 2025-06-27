package user

import (
	"time"
)

// UserEntity 用户数据库实体
// 对应数据库表结构
type UserEntity struct {
	ID        string    `gorm:"primaryKey;column:id" json:"id"`
	Username  string    `gorm:"uniqueIndex;column:username;type:varchar(50)" json:"username"`
	Email     string    `gorm:"uniqueIndex;column:email;type:varchar(100)" json:"email"`
	Password  string    `gorm:"column:password;type:varchar(255)" json:"-"`
	Status    int       `gorm:"column:status;type:int;default:1" json:"status"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (UserEntity) TableName() string {
	return "users"
}
