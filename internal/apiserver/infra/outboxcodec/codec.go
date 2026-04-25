package outboxcodec

import (
	"github.com/FangcunMount/qs-server/internal/pkg/eventcodec"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// Encode serializes a domain event into a generic payload envelope.
func Encode(evt event.DomainEvent) (string, error) {
	payload, err := eventcodec.EncodeDomainEvent(evt)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

// Decode deserializes a generic payload envelope back into a domain event.
func Decode(payload string) (event.DomainEvent, error) {
	return eventcodec.DecodeDomainEvent([]byte(payload))
}
