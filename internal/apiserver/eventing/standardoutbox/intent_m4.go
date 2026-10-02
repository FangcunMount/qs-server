package standardoutbox

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/event"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/reliable-messaging/message"
	domainwire "github.com/FangcunMount/reliable-messaging/wire/domain"
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
	prepared := make([]PreparedIntent, 0, len(events))
	if len(events) == 0 {
		return prepared, nil
	}
	dueAt := time.Now()
	for _, evt := range events {
		if evt == nil {
			return nil, fmt.Errorf("domain event is nil")
		}
		topic, found := resolver.GetTopicForEvent(evt.EventType())
		if !found {
			return nil, fmt.Errorf("event %q not found in event config", evt.EventType())
		}
		if deliveryResolver, ok := resolver.(eventcatalog.DeliveryClassResolver); ok {
			delivery, found := deliveryResolver.GetDeliveryClass(evt.EventType())
			if !found {
				return nil, fmt.Errorf("event %q has no delivery class", evt.EventType())
			}
			if delivery != eventcatalog.DeliveryClassDurableOutbox {
				return nil, fmt.Errorf("event %q delivery class %q cannot be staged to outbox", evt.EventType(), delivery)
			}
		}
		payload, err := domainwire.EncodeEvent(evt)
		if err != nil {
			return nil, err
		}
		orgID := organizationID(payload)
		if orgID == nil || *orgID <= 0 {
			return nil, fmt.Errorf("event %q has no valid organization scope", evt.EventID())
		}
		wire, err := EncodeWire(evt, source)
		if err != nil {
			return nil, err
		}
		intent, err := message.New(message.Input{
			Producer: "qs-server", ID: evt.EventID(), Destination: topic,
			EventType: evt.EventType(), SchemaVersion: "v1", Scope: fmt.Sprintf("org:%d", *orgID),
			ContentType: "application/json", OccurredAt: evt.OccurredAt().In(messageTimeZone).Format(time.RFC3339Nano), Payload: wire,
		})
		if err != nil {
			return nil, err
		}
		prepared = append(prepared, PreparedIntent{Message: intent, DueAt: dueAt})
	}
	return prepared, nil
}

// organizationID is QS's business scope extraction; the SDK owns the JSON
// event envelope, while the host decides how an organization is represented.
func organizationID(payload []byte) *int64 {
	envelope, err := domainwire.DecodeEnvelope(payload)
	if err != nil {
		return nil
	}
	var data struct {
		OrgID int64 `json:"org_id"`
	}
	if json.Unmarshal(envelope.Data, &data) != nil || data.OrgID == 0 {
		return nil
	}
	return &data.OrgID
}
