package mysql

import "time"

// AuditFields 用于统一管理 ID、创建时间和更新时间
type AuditFields struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

// Syncable 定义所有支持自动回填的实体结构
type Syncable interface {
	GetID() uint64
	GetCreatedAt() time.Time
	GetUpdatedAt() time.Time
	SetID(uint64)
	SetCreatedAt(time.Time)
	SetUpdatedAt(time.Time)
}
