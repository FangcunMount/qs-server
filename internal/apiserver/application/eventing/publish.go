package eventing

import (
	"context"

	"github.com/FangcunMount/qs-server/pkg/event"
)

// Source 表示在应用层暂存领域事件的对象。
type Source interface {
	Events() []event.DomainEvent
	ClearEvents()
}

// FailureHandler 在单个事件发布失败时被调用。
type FailureHandler func(event.DomainEvent, error)

// MissingPublisherHandler 在未配置事件发布器时被调用。
type MissingPublisherHandler func()

type staticSource struct {
	events []event.DomainEvent
}

// Collect 将一组事件包装为共享发布 helper 可消费的 Source。
func Collect(events ...event.DomainEvent) Source {
	return &staticSource{events: events}
}

func (s *staticSource) Events() []event.DomainEvent {
	return s.events
}

func (s *staticSource) ClearEvents() {
	s.events = nil
}

// PublishCollectedEvents 发布 Source 中暂存的领域事件。
// 发布失败不会中断剩余事件的尝试，且只有在遍历完成后才清空事件。
func PublishCollectedEvents(
	ctx context.Context,
	publisher event.EventPublisher,
	source Source,
	onMissingPublisher MissingPublisherHandler,
	onFailure FailureHandler,
) {
	if source == nil {
		return
	}
	if publisher == nil {
		if onMissingPublisher != nil {
			onMissingPublisher()
		}
		return
	}

	for _, evt := range source.Events() {
		if err := publisher.Publish(ctx, evt); err != nil && onFailure != nil {
			onFailure(evt, err)
		}
	}
	source.ClearEvents()
}
