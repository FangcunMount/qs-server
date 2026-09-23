//go:build reliable_messaging_m4

package standardoutbox

import (
	"context"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	"github.com/FangcunMount/qs-server/internal/apiserver/outboxcore"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/reliable-messaging/message"
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

var messageTimeZone = time.FixedZone("UTC+8", 8*60*60)

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
	// Reuse the existing catalog's durable-only routing and canonical event
	// encoding. The SDK payload is the full original NSQ wire envelope.
	records, err := outboxcore.BuildRecords(outboxcore.BuildRecordsOptions{Events: events, Resolver: s.resolver})
	if err != nil {
		return err
	}
	for index, record := range records {
		orgID := outboxcore.OrgIDFromPayloadJSON(record.PayloadJSON)
		if orgID == nil || *orgID <= 0 {
			return fmt.Errorf("event %q has no valid organization scope", record.EventID)
		}
		wire, err := standardoutbox.EncodeWire(events[index], s.source)
		if err != nil {
			return err
		}
		intent, err := message.New(message.Input{
			Producer: "qs-server", ID: record.EventID, Destination: record.TopicName,
			EventType: record.EventType, SchemaVersion: "v1", Scope: fmt.Sprintf("org:%d", *orgID),
			ContentType: "application/json", OccurredAt: events[index].OccurredAt().In(messageTimeZone).Format(time.RFC3339Nano), Payload: wire,
		})
		if err != nil {
			return err
		}
		if err := appender.Append(intent, record.NextAttemptAt); err != nil {
			return err
		}
	}
	return nil
}
