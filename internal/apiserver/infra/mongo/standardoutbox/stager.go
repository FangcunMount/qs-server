//go:build reliable_messaging_m4

package standardoutbox

import (
	"context"
	"fmt"

	"github.com/FangcunMount/component-base/pkg/event"
	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	sdkmongo "github.com/FangcunMount/reliable-messaging/storage/mongo"
	"go.mongodb.org/mongo-driver/mongo"
)

// Stager borrows the host's active Mongo transaction. It never owns a session
// or starts a second transaction, so AnswerSheet and its intent commit together.
type Stager struct {
	collection *mongo.Collection
	resolver   eventcatalog.TopicResolver
	source     string
}

func NewStager(collection *mongo.Collection, resolver eventcatalog.TopicResolver, source string) (*Stager, error) {
	if collection == nil || resolver == nil || source == "" {
		return nil, fmt.Errorf("standard Mongo stager requires collection, topic resolver and source")
	}
	return &Stager{collection: collection, resolver: resolver, source: source}, nil
}

func (s *Stager) Stage(ctx context.Context, events ...event.DomainEvent) error {
	if s == nil || s.collection == nil || s.resolver == nil || s.source == "" {
		return fmt.Errorf("standard Mongo stager is not configured")
	}
	if len(events) == 0 {
		return nil
	}
	txCtx, ok := ctx.(mongo.SessionContext)
	if !ok {
		return sdkmongo.ErrTransactionRequired
	}
	appender, err := sdkmongo.Bind(txCtx, s.collection)
	if err != nil {
		return err
	}
	prepared, err := standardoutbox.PrepareIntents(events, s.resolver, s.source)
	if err != nil {
		return err
	}
	for _, item := range prepared {
		if err := appender.Append(item.Message, item.DueAt); err != nil {
			return err
		}
	}
	return nil
}
