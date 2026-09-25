//go:build reliable_messaging_m4

package standardoutbox

import (
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
	"github.com/FangcunMount/qs-server/internal/apiserver/outboxcore"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/reliable-messaging/message"
)

var messageTimeZone = time.FixedZone("UTC+8", 8*60*60)

type PreparedIntent struct {
	Message message.Message
	DueAt   time.Time
}

// PrepareIntents keeps QS routing, organization scope and original NSQ wire
// identical across its MySQL and Mongo host-owned transactions.
func PrepareIntents(events []event.DomainEvent, resolver eventcatalog.TopicResolver, source string) ([]PreparedIntent, error) {
	if resolver == nil || source == "" {
		return nil, fmt.Errorf("topic resolver and source required")
	}
	records, err := outboxcore.BuildRecords(outboxcore.BuildRecordsOptions{Events: events, Resolver: resolver})
	if err != nil {
		return nil, err
	}
	prepared := make([]PreparedIntent, 0, len(records))
	for index, record := range records {
		orgID := outboxcore.OrgIDFromPayloadJSON(record.PayloadJSON)
		if orgID == nil || *orgID <= 0 {
			return nil, fmt.Errorf("event %q has no valid organization scope", record.EventID)
		}
		wire, err := EncodeWire(events[index], source)
		if err != nil {
			return nil, err
		}
		intent, err := message.New(message.Input{
			Producer: "qs-server", ID: record.EventID, Destination: record.TopicName,
			EventType: record.EventType, SchemaVersion: "v1", Scope: fmt.Sprintf("org:%d", *orgID),
			ContentType: "application/json", OccurredAt: events[index].OccurredAt().In(messageTimeZone).Format(time.RFC3339Nano), Payload: wire,
		})
		if err != nil {
			return nil, err
		}
		prepared = append(prepared, PreparedIntent{Message: intent, DueAt: record.NextAttemptAt})
	}
	return prepared, nil
}
