package eventruntime

import (
	"encoding/json"

	"github.com/FangcunMount/qs-server/internal/pkg/event"
	domainwire "github.com/FangcunMount/reliable-messaging/wire/domain"
)

// DecodeDomainEvent uses the SDK wire parser and restores a QS-owned event
// value. Raw data retains large identifiers and the historical replay format.
func DecodeDomainEvent(payload []byte) (event.DomainEvent, error) {
	envelope, err := domainwire.DecodeEnvelope(payload)
	if err != nil {
		return nil, err
	}
	return event.Event[json.RawMessage]{
		BaseEvent: event.BaseEvent{
			ID: envelope.ID, EventTypeValue: envelope.EventType,
			OccurredAtValue:    envelope.OccurredAt,
			AggregateTypeValue: envelope.AggregateType, AggregateIDValue: envelope.AggregateID,
		},
		Data: envelope.Data,
	}, nil
}
