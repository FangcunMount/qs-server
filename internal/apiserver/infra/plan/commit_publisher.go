package plan

import (
	"context"
	"github.com/FangcunMount/component-base/pkg/event"
	gormuow "github.com/FangcunMount/component-base/pkg/uow/gorm"
)

// NewCommitPublisher defers plan notifications until the outermost MySQL
// transaction commits. It does not replace durable Outbox delivery.
func NewCommitPublisher(next event.EventPublisher) event.EventPublisher {
	if next == nil {
		return nil
	}
	return commitPublisher{next: next}
}

type commitPublisher struct{ next event.EventPublisher }

func (p commitPublisher) Publish(ctx context.Context, evt event.DomainEvent) error {
	if _, active := gormuow.TxFromContext(ctx); active {
		return gormuow.AfterCommit(ctx, func(committed context.Context) error { return p.next.Publish(committed, evt) })
	}
	return p.next.Publish(ctx, evt)
}
func (p commitPublisher) PublishAll(ctx context.Context, events []event.DomainEvent) error {
	for _, evt := range events {
		if err := p.Publish(ctx, evt); err != nil {
			return err
		}
	}
	return nil
}
