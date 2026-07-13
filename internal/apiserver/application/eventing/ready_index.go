package eventing

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
)

// ReadyIndex 是best-effort Redis ZSet 调度器 用于 待处理 outbox 事件。
type ReadyIndex interface {
	Enqueue(ctx context.Context, eventType, eventID string, nextAttemptAt, createdAt time.Time) error
	Remove(ctx context.Context, eventType, eventID string) error
	RemoveByEventID(ctx context.Context, eventID string) error
	ClaimDueIDs(ctx context.Context, bucket string, limit int, now time.Time) ([]string, error)
}

// PostCommitReadyIndexer 回填就绪索引 在之后 outbox 行 是 已提交。
type PostCommitReadyIndexer struct {
	index ReadyIndex
}

func NewPostCommitReadyIndexer(index ReadyIndex) *PostCommitReadyIndexer {
	if index == nil {
		return nil
	}
	return &PostCommitReadyIndexer{index: index}
}

func (p *PostCommitReadyIndexer) EnqueueAfterCommit(ctx context.Context, events []event.DomainEvent, nextAttemptAt time.Time) {
	if p == nil || p.index == nil || len(events) == 0 {
		return
	}
	if nextAttemptAt.IsZero() {
		nextAttemptAt = time.Now()
	}
	for _, evt := range events {
		if evt == nil {
			continue
		}
		createdAt := evt.OccurredAt()
		if createdAt.IsZero() {
			createdAt = nextAttemptAt
		}
		_ = p.index.Enqueue(ctx, evt.EventType(), evt.EventID(), nextAttemptAt, createdAt)
	}
}
