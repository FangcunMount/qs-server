package definition

import (
	"strconv"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventpayload"
	"github.com/FangcunMount/qs-server/pkg/event"
)

const (
	// EventTypeChanged 量表生命周期变化
	EventTypeChanged = eventcatalog.ScaleChanged
)

// AggregateType 聚合根类型
const AggregateType = "MedicalScale"

// ChangeAction 量表生命周期动作
type ChangeAction = eventpayload.ScaleChangeAction

const (
	ChangeActionPublished   = eventpayload.ScaleChangeActionPublished
	ChangeActionUnpublished = eventpayload.ScaleChangeActionUnpublished
	ChangeActionUpdated     = eventpayload.ScaleChangeActionUpdated
	ChangeActionArchived    = eventpayload.ScaleChangeActionArchived
)

// ScaleChangedData 量表生命周期变化事件数据
type ScaleChangedData = eventpayload.ScaleChangedData

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
