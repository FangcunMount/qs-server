package questionnaire

import (
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
)

const (
	// EventTypeChanged 问卷生命周期变化
	EventTypeChanged = eventcatalog.QuestionnaireChanged
)

// AggregateType 聚合根类型
const AggregateType = "Questionnaire"

// ChangeAction 问卷生命周期动作
type ChangeAction = eventpayload.QuestionnaireChangeAction

const (
	ChangeActionPublished   = eventpayload.QuestionnaireChangeActionPublished
	ChangeActionUnpublished = eventpayload.QuestionnaireChangeActionUnpublished
	ChangeActionArchived    = eventpayload.QuestionnaireChangeActionArchived
)

// QuestionnaireChangedData 问卷生命周期变化事件数据
type QuestionnaireChangedData = eventpayload.QuestionnaireChangedData

// QuestionnaireChangedEvent 问卷生命周期变化事件
type QuestionnaireChangedEvent = event.Event[QuestionnaireChangedData]

// NewQuestionnaireChangedEvent 创建问卷生命周期变化事件
func NewQuestionnaireChangedEvent(
	code string,
	version string,
	title string,
	action ChangeAction,
	changedAt time.Time,
) QuestionnaireChangedEvent {
	return event.New(EventTypeChanged, AggregateType, code,
		QuestionnaireChangedData{
			Code:      code,
			Version:   version,
			Title:     title,
			Action:    action,
			ChangedAt: changedAt,
		},
	)
}
