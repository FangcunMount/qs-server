package assessment

import (
	"context"

	"github.com/FangcunMount/qs-server/pkg/event"
)

type domainEventCollector interface {
	Events() []event.DomainEvent
	ClearEvents()
}

// PublishCollectedEvents 发布聚合根暂存的领域事件并在完成后清空事件列表。
// 发布失败不会中断剩余事件的尝试，由调用方通过 onFailure 记录日志。
func PublishCollectedEvents(
	ctx context.Context,
	publisher event.EventPublisher,
	collector domainEventCollector,
	onFailure func(event.DomainEvent, error),
) {
	if publisher == nil || collector == nil {
		return
	}

	events := collector.Events()
	for _, evt := range events {
		if err := publisher.Publish(ctx, evt); err != nil && onFailure != nil {
			onFailure(evt, err)
		}
	}
	collector.ClearEvents()
}
