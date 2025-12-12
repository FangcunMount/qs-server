package scale

import (
	"strconv"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/eventconfig"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// ==================== 事件类型常量 ====================
// 从 eventconfig 包导入，保持事件类型的单一来源

const (
	// EventTypePublished 量表已发布
	EventTypePublished = eventconfig.ScalePublished
	// EventTypeUnpublished 量表已下架
	EventTypeUnpublished = eventconfig.ScaleUnpublished
	// EventTypeUpdated 量表已更新
	EventTypeUpdated = eventconfig.ScaleUpdated
	// EventTypeArchived 量表已归档
	EventTypeArchived = eventconfig.ScaleArchived
)

// AggregateType 聚合根类型
const AggregateType = "MedicalScale"

// ==================== 事件 Payload 定义 ====================

// ScalePublishedData 量表已发布事件数据
type ScalePublishedData struct {
	ScaleID     uint64    `json:"scale_id"`
	Code        string    `json:"code"`
	Version     string    `json:"version"`
	Name        string    `json:"name"`
	PublishedAt time.Time `json:"published_at"`
}

// ScaleUnpublishedData 量表已下架事件数据
type ScaleUnpublishedData struct {
	ScaleID       uint64    `json:"scale_id"`
	Code          string    `json:"code"`
	Version       string    `json:"version"`
	UnpublishedAt time.Time `json:"unpublished_at"`
}

// ScaleUpdatedData 量表已更新事件数据
type ScaleUpdatedData struct {
	ScaleID   uint64    `json:"scale_id"`
	Code      string    `json:"code"`
	Version   string    `json:"version"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ScaleArchivedData 量表已归档事件数据
type ScaleArchivedData struct {
	ScaleID    uint64    `json:"scale_id"`
	Code       string    `json:"code"`
	Version    string    `json:"version"`
	ArchivedAt time.Time `json:"archived_at"`
}

// ==================== 事件类型别名 ====================

// ScalePublishedEvent 量表已发布事件
type ScalePublishedEvent = event.Event[ScalePublishedData]

// ScaleUnpublishedEvent 量表已下架事件
type ScaleUnpublishedEvent = event.Event[ScaleUnpublishedData]

// ScaleUpdatedEvent 量表已更新事件
type ScaleUpdatedEvent = event.Event[ScaleUpdatedData]

// ScaleArchivedEvent 量表已归档事件
type ScaleArchivedEvent = event.Event[ScaleArchivedData]

// ==================== 事件构造函数 ====================

// NewScalePublishedEvent 创建量表已发布事件
func NewScalePublishedEvent(
	scaleID uint64,
	code string,
	version string,
	name string,
	publishedAt time.Time,
) ScalePublishedEvent {
	return event.New(EventTypePublished, AggregateType, strconv.FormatUint(scaleID, 10),
		ScalePublishedData{
			ScaleID:     scaleID,
			Code:        code,
			Version:     version,
			Name:        name,
			PublishedAt: publishedAt,
		},
	)
}

// NewScaleUnpublishedEvent 创建量表已下架事件
func NewScaleUnpublishedEvent(
	scaleID uint64,
	code string,
	version string,
	unpublishedAt time.Time,
) ScaleUnpublishedEvent {
	return event.New(EventTypeUnpublished, AggregateType, strconv.FormatUint(scaleID, 10),
		ScaleUnpublishedData{
			ScaleID:       scaleID,
			Code:          code,
			Version:       version,
			UnpublishedAt: unpublishedAt,
		},
	)
}

// NewScaleUpdatedEvent 创建量表已更新事件
func NewScaleUpdatedEvent(
	scaleID uint64,
	code string,
	version string,
	updatedAt time.Time,
) ScaleUpdatedEvent {
	return event.New(EventTypeUpdated, AggregateType, strconv.FormatUint(scaleID, 10),
		ScaleUpdatedData{
			ScaleID:   scaleID,
			Code:      code,
			Version:   version,
			UpdatedAt: updatedAt,
		},
	)
}

// NewScaleArchivedEvent 创建量表已归档事件
func NewScaleArchivedEvent(
	scaleID uint64,
	code string,
	version string,
	archivedAt time.Time,
) ScaleArchivedEvent {
	return event.New(EventTypeArchived, AggregateType, strconv.FormatUint(scaleID, 10),
		ScaleArchivedData{
			ScaleID:    scaleID,
			Code:       code,
			Version:    version,
			ArchivedAt: archivedAt,
		},
	)
}
