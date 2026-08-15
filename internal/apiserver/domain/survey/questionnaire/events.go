package questionnaire

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"github.com/google/uuid"
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

// LifecycleEventMetadata fixes the event identity before a Mongo transaction
// callback starts. Mongo may execute that callback more than once, so creating
// either value inside the callback would make retries observably different.
type LifecycleEventMetadata struct {
	EventID    string
	OccurredAt time.Time
}

type lifecycleEventMetadataContextKey struct{}

// NewLifecycleEventMetadata allocates retry-stable metadata before entering a
// transaction callback.
func NewLifecycleEventMetadata(occurredAt time.Time) LifecycleEventMetadata {
	return LifecycleEventMetadata{EventID: uuid.NewString(), OccurredAt: occurredAt.UTC()}
}

// WithLifecycleEventMetadata keeps an existing outer transaction's metadata,
// or allocates it once for a standalone lifecycle transaction.
func WithLifecycleEventMetadata(ctx context.Context, occurredAt time.Time) context.Context {
	if _, ok := LifecycleEventMetadataFromContext(ctx); ok {
		return ctx
	}
	return context.WithValue(ctx, lifecycleEventMetadataContextKey{}, NewLifecycleEventMetadata(occurredAt))
}

// LifecycleEventMetadataFromContext returns retry-stable event metadata.
func LifecycleEventMetadataFromContext(ctx context.Context) (LifecycleEventMetadata, bool) {
	if ctx == nil {
		return LifecycleEventMetadata{}, false
	}
	metadata, ok := ctx.Value(lifecycleEventMetadataContextKey{}).(LifecycleEventMetadata)
	return metadata, ok && metadata.EventID != "" && !metadata.OccurredAt.IsZero()
}

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

func newQuestionnaireChangedEventWithMetadata(
	metadata LifecycleEventMetadata,
	code string,
	version string,
	title string,
	action ChangeAction,
) QuestionnaireChangedEvent {
	return QuestionnaireChangedEvent{
		BaseEvent: event.BaseEvent{
			ID:                 metadata.EventID,
			EventTypeValue:     EventTypeChanged,
			OccurredAtValue:    metadata.OccurredAt,
			AggregateTypeValue: AggregateType,
			AggregateIDValue:   code,
		},
		Data: QuestionnaireChangedData{
			Code:      code,
			Version:   version,
			Title:     title,
			Action:    action,
			ChangedAt: metadata.OccurredAt,
		},
	}
}
