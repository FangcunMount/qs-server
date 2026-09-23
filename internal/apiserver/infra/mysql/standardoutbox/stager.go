//go:build reliable_messaging_m4

package standardoutbox

import (
	"context"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	sdkmysql "github.com/FangcunMount/reliable-messaging/storage/mysql"
)

// Stager writes standard messages only through the host's existing GORM
// transaction, including scheduled events from the same business boundary.
type Stager struct {
	resolver eventcatalog.TopicResolver
	source   string
}

func NewStager(resolver eventcatalog.TopicResolver, source string) (*Stager, error) {
	if resolver == nil || source == "" {
		return nil, fmt.Errorf("standard MySQL stager requires topic resolver and source")
	}
	return &Stager{resolver: resolver, source: source}, nil
}

func (s *Stager) Stage(ctx context.Context, events ...event.DomainEvent) error {
	return s.stageAt(ctx, time.Time{}, events)
}

func (s *Stager) StageAt(ctx context.Context, dueAt time.Time, events ...event.DomainEvent) error {
	if dueAt.IsZero() {
		return fmt.Errorf("scheduled standard message requires due time")
	}
	return s.stageAt(ctx, dueAt, events)
}

func (s *Stager) stageAt(ctx context.Context, dueAt time.Time, events []event.DomainEvent) error {
	if s == nil || s.resolver == nil || s.source == "" {
		return fmt.Errorf("standard MySQL stager is not configured")
	}
	if len(events) == 0 {
		return nil
	}
	tx, err := mysql.RequireTx(ctx)
	if err != nil {
		return err
	}
	appender, err := sdkmysql.BindGORM(tx)
	if err != nil {
		return err
	}
	prepared, err := standardoutbox.PrepareIntents(events, s.resolver, s.source)
	if err != nil {
		return err
	}
	for _, item := range prepared {
		when := item.DueAt
		if !dueAt.IsZero() {
			when = dueAt
		}
		if err := appender.Append(ctx, item.Message, when); err != nil {
			return err
		}
	}
	return nil
}
