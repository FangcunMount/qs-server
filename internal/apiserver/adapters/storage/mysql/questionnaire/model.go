package questionnaire

import (
	"time"
)

// Model MySQL 问卷表模型
// 专门用于数据库映射，包含 GORM 标签
type Model struct {
	ID          string    `gorm:"primaryKey" json:"id"`
	Code        string    `gorm:"uniqueIndex" json:"code"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Status      int       `json:"status"`
	CreatedBy   string    `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Version     int       `json:"version"`
}

// TableName 指定表名
func (Model) TableName() string {
	return "questionnaires"
}
