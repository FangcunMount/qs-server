package mysql

import "time"

// Syncable 定义所有支持自动回填的实体结构
type Syncable interface {
	GetID() uint64
	GetCreatedAt() time.Time
	GetUpdatedAt() time.Time
	GetDeletedAt() time.Time
	GetCreatedBy() uint64
	GetUpdatedBy() uint64
	GetDeletedBy() uint64
	SetID(uint64)
	SetCreatedAt(time.Time)
	SetUpdatedAt(time.Time)
	SetDeletedAt(time.Time)
	SetCreatedBy(uint64)
	SetUpdatedBy(uint64)
	SetDeletedBy(uint64)
}

// AuditFields 用于统一管理 ID、创建时间和更新时间
type AuditFields struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt time.Time `gorm:"column:deleted_at;index"`
	CreatedBy uint64    `gorm:"column:created_by;type:varchar(50)" json:"created_by"`
	UpdatedBy uint64    `gorm:"column:updated_by;type:varchar(50)" json:"updated_by"`
	DeletedBy uint64    `gorm:"column:deleted_by;type:varchar(50)" json:"deleted_by"`
}

func (a *AuditFields) GetID() uint64 {
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

func (a *AuditFields) GetCreatedBy() uint64 {
	return a.CreatedBy
}

func (a *AuditFields) GetUpdatedBy() uint64 {
	return a.UpdatedBy
}

func (a *AuditFields) GetDeletedBy() uint64 {
	return a.DeletedBy
}

func (a *AuditFields) SetID(id uint64) {
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

func (a *AuditFields) SetCreatedBy(id uint64) {
	a.CreatedBy = id
}

func (a *AuditFields) SetUpdatedBy(id uint64) {
	a.UpdatedBy = id
}

func (a *AuditFields) SetDeletedBy(id uint64) {
	a.DeletedBy = id
}
