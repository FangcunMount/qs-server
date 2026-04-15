package questionnaire

import (
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/eventconfig"
	"github.com/FangcunMount/qs-server/pkg/event"
)

const (
	// EventTypeChanged 问卷生命周期变化
	EventTypeChanged = eventconfig.QuestionnaireChanged
)

// AggregateType 聚合根类型
const AggregateType = "Questionnaire"

// ChangeAction 问卷生命周期动作
type ChangeAction string

const (
	ChangeActionPublished   ChangeAction = "published"
	ChangeActionUnpublished ChangeAction = "unpublished"
	ChangeActionArchived    ChangeAction = "archived"
)

// QuestionnaireChangedData 问卷生命周期变化事件数据
type QuestionnaireChangedData struct {
	Code      string       `json:"code"`
	Version   string       `json:"version"`
	Title     string       `json:"title"`
	Action    ChangeAction `json:"action"`
	ChangedAt time.Time    `json:"changed_at"`
}

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
