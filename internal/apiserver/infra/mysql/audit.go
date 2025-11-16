package mysql

import (
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// Syncable 定义所有支持自动回填的实体结构
type Syncable interface {
	GetID() meta.ID
	GetCreatedAt() time.Time
	GetUpdatedAt() time.Time
	GetDeletedAt() time.Time
	GetCreatedBy() meta.ID
	GetUpdatedBy() meta.ID
	GetDeletedBy() meta.ID
	SetID(meta.ID)
	SetCreatedAt(time.Time)
	SetUpdatedAt(time.Time)
	SetDeletedAt(time.Time)
	SetCreatedBy(meta.ID)
	SetUpdatedBy(meta.ID)
	SetDeletedBy(meta.ID)
}

// AuditFields 用于统一管理 ID、创建时间和更新时间
type AuditFields struct {
	ID        meta.ID   `gorm:"primaryKey;autoIncrement"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt time.Time `gorm:"column:deleted_at;index"`
	CreatedBy meta.ID   `gorm:"column:created_by;type:varchar(50)" json:"created_by"`
	UpdatedBy meta.ID   `gorm:"column:updated_by;type:varchar(50)" json:"updated_by"`
	DeletedBy meta.ID   `gorm:"column:deleted_by;type:varchar(50)" json:"deleted_by"`
}

func (a *AuditFields) GetID() meta.ID {
	return a.ID
}

func (a *AuditFields) GetCreatedAt() time.Time {
	return a.CreatedAt
}

func (a *AuditFields) GetUpdatedAt() time.Time {
	return a.UpdatedAt
}

func (a *AuditFields) GetDeletedAt() time.Time {
	return a.DeletedAt
}

func (a *AuditFields) GetCreatedBy() meta.ID {
	return a.CreatedBy
}

func (a *AuditFields) GetUpdatedBy() meta.ID {
	return a.UpdatedBy
}

func (a *AuditFields) GetDeletedBy() meta.ID {
	return a.DeletedBy
}

func (a *AuditFields) SetID(id meta.ID) {
	a.ID = id
}

func (a *AuditFields) SetCreatedAt(t time.Time) {
	a.CreatedAt = t
}

func (a *AuditFields) SetUpdatedAt(t time.Time) {
	a.UpdatedAt = t
}

func (a *AuditFields) SetDeletedAt(t time.Time) {
	a.DeletedAt = t
}

func (a *AuditFields) SetCreatedBy(id meta.ID) {
	a.CreatedBy = id
}

func (a *AuditFields) SetUpdatedBy(id meta.ID) {
	a.UpdatedBy = id
}

func (a *AuditFields) SetDeletedBy(id meta.ID) {
	a.DeletedBy = id
}
