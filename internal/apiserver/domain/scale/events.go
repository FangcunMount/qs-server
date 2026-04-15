package scale

import (
	"strconv"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/eventconfig"
	"github.com/FangcunMount/qs-server/pkg/event"
)

const (
	// EventTypeChanged 量表生命周期变化
	EventTypeChanged = eventconfig.ScaleChanged
)

// AggregateType 聚合根类型
const AggregateType = "MedicalScale"

// ChangeAction 量表生命周期动作
type ChangeAction string

const (
	ChangeActionPublished   ChangeAction = "published"
	ChangeActionUnpublished ChangeAction = "unpublished"
	ChangeActionUpdated     ChangeAction = "updated"
	ChangeActionArchived    ChangeAction = "archived"
)

// ScaleChangedData 量表生命周期变化事件数据
type ScaleChangedData struct {
	ScaleID   uint64       `json:"scale_id"`
	Code      string       `json:"code"`
	Version   string       `json:"version"`
	Name      string       `json:"name"`
	Action    ChangeAction `json:"action"`
	ChangedAt time.Time    `json:"changed_at"`
}

// ScaleChangedEvent 量表生命周期变化事件
type ScaleChangedEvent = event.Event[ScaleChangedData]

// NewScaleChangedEvent 创建量表生命周期变化事件
func NewScaleChangedEvent(
	scaleID uint64,
	code string,
	version string,
	name string,
	action ChangeAction,
	changedAt time.Time,
) ScaleChangedEvent {
	return event.New(EventTypeChanged, AggregateType, strconv.FormatUint(scaleID, 10),
		ScaleChangedData{
			ScaleID:   scaleID,
			Code:      code,
			Version:   version,
			Name:      name,
			Action:    action,
			ChangedAt: changedAt,
		},
	)
}
