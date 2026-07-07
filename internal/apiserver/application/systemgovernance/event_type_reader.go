package systemgovernance

import (
	"context"
	"time"

	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
)

// ReadEventTypes 加载按事件类型 积压 行。
func ReadEventTypes(ctx context.Context, sources []EventTypeStatusSource, now time.Time) []EventTypeStatusGroup {
	groups := make([]EventTypeStatusGroup, 0, len(sources))
	for _, source := range sources {
		group := EventTypeStatusGroup{Store: source.Store}
		if source.Reader == nil {
			group.Error = "event type status reader unavailable"
			groups = append(groups, group)
			continue
		}
		buckets, err := source.Reader.OutboxStatusByEventType(ctx, now)
		if err != nil {
			group.Error = err.Error()
			groups = append(groups, group)
			continue
		}
		group.Buckets = append([]outboxport.EventTypeStatusBucket(nil), buckets...)
		groups = append(groups, group)
	}
	return groups
}
