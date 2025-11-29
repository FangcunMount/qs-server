package scale

import (
	"time"

	"github.com/FangcunMount/qs-server/pkg/event"
)

// ==================== ScalePublishedEvent ====================

// ScalePublishedEvent 量表已发布事件
// 用途：
// - collection-server 更新量表缓存
// - qs-worker 预加载计算规则
type ScalePublishedEvent struct {
	event.BaseEvent

	scaleID     uint64
	code        string
	version     string
	name        string
	publishedAt time.Time
}

// NewScalePublishedEvent 创建量表已发布事件
func NewScalePublishedEvent(
	scaleID uint64,
	code string,
	version string,
	name string,
	publishedAt time.Time,
) *ScalePublishedEvent {
	return &ScalePublishedEvent{
		BaseEvent:   event.NewBaseEvent("scale.published", "MedicalScale", code),
		scaleID:     scaleID,
		code:        code,
		version:     version,
		name:        name,
		publishedAt: publishedAt,
	}
}

// ScaleID 获取量表ID
func (e *ScalePublishedEvent) ScaleID() uint64 {
	return e.scaleID
}

// Code 获取量表编码
func (e *ScalePublishedEvent) Code() string {
	return e.code
}

// Version 获取量表版本
func (e *ScalePublishedEvent) Version() string {
	return e.version
}

// Name 获取量表名称
func (e *ScalePublishedEvent) Name() string {
	return e.name
}

// PublishedAt 获取发布时间
func (e *ScalePublishedEvent) PublishedAt() time.Time {
	return e.publishedAt
}

// ==================== ScaleUnpublishedEvent ====================

// ScaleUnpublishedEvent 量表已下架事件
// 用途：
// - collection-server 清除量表缓存
// - qs-worker 清除计算规则缓存
type ScaleUnpublishedEvent struct {
	event.BaseEvent

	scaleID       uint64
	code          string
	version       string
	unpublishedAt time.Time
}

// NewScaleUnpublishedEvent 创建量表已下架事件
func NewScaleUnpublishedEvent(
	scaleID uint64,
	code string,
	version string,
	unpublishedAt time.Time,
) *ScaleUnpublishedEvent {
	return &ScaleUnpublishedEvent{
		BaseEvent:     event.NewBaseEvent("scale.unpublished", "MedicalScale", code),
		scaleID:       scaleID,
		code:          code,
		version:       version,
		unpublishedAt: unpublishedAt,
	}
}

// ScaleID 获取量表ID
func (e *ScaleUnpublishedEvent) ScaleID() uint64 {
	return e.scaleID
}

// Code 获取量表编码
func (e *ScaleUnpublishedEvent) Code() string {
	return e.code
}

// Version 获取量表版本
func (e *ScaleUnpublishedEvent) Version() string {
	return e.version
}

// UnpublishedAt 获取下架时间
func (e *ScaleUnpublishedEvent) UnpublishedAt() time.Time {
	return e.unpublishedAt
}

// ==================== ScaleUpdatedEvent ====================

// ScaleUpdatedEvent 量表已更新事件
// 用途：
// - qs-worker 重新加载计算规则
// - 当量表的因子、解读规则等发生变化时触发
type ScaleUpdatedEvent struct {
	event.BaseEvent

	scaleID   uint64
	code      string
	version   string
	updatedAt time.Time
}

// NewScaleUpdatedEvent 创建量表已更新事件
func NewScaleUpdatedEvent(
	scaleID uint64,
	code string,
	version string,
	updatedAt time.Time,
) *ScaleUpdatedEvent {
	return &ScaleUpdatedEvent{
		BaseEvent: event.NewBaseEvent("scale.updated", "MedicalScale", code),
		scaleID:   scaleID,
		code:      code,
		version:   version,
		updatedAt: updatedAt,
	}
}

// ScaleID 获取量表ID
func (e *ScaleUpdatedEvent) ScaleID() uint64 {
	return e.scaleID
}

// Code 获取量表编码
func (e *ScaleUpdatedEvent) Code() string {
	return e.code
}

// Version 获取量表版本
func (e *ScaleUpdatedEvent) Version() string {
	return e.version
}

// UpdatedAt 获取更新时间
func (e *ScaleUpdatedEvent) UpdatedAt() time.Time {
	return e.updatedAt
}

// ==================== ScaleArchivedEvent ====================

// ScaleArchivedEvent 量表已归档事件
// 用途：
// - collection-server 清除缓存
// - qs-worker 清除计算规则缓存
type ScaleArchivedEvent struct {
	event.BaseEvent

	scaleID    uint64
	code       string
	version    string
	archivedAt time.Time
}

// NewScaleArchivedEvent 创建量表已归档事件
func NewScaleArchivedEvent(
	scaleID uint64,
	code string,
	version string,
	archivedAt time.Time,
) *ScaleArchivedEvent {
	return &ScaleArchivedEvent{
		BaseEvent:  event.NewBaseEvent("scale.archived", "MedicalScale", code),
		scaleID:    scaleID,
		code:       code,
		version:    version,
		archivedAt: archivedAt,
	}
}

// ScaleID 获取量表ID
func (e *ScaleArchivedEvent) ScaleID() uint64 {
	return e.scaleID
}

// Code 获取量表编码
func (e *ScaleArchivedEvent) Code() string {
	return e.code
}

// Version 获取量表版本
func (e *ScaleArchivedEvent) Version() string {
	return e.version
}

// ArchivedAt 获取归档时间
func (e *ScaleArchivedEvent) ArchivedAt() time.Time {
	return e.archivedAt
}
