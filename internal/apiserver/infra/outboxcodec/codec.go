package outboxcodec

import (
	"encoding/json"

	"github.com/FangcunMount/qs-server/pkg/event"
)

type storedDomainEvent struct {
	event.BaseEvent
	Data json.RawMessage `json:"data"`
}

// Encode serializes a domain event into a generic payload envelope.
func Encode(evt event.DomainEvent) (string, error) {
	payload, err := json.Marshal(evt)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

// Decode deserializes a generic payload envelope back into a domain event.
func Decode(payload string) (event.DomainEvent, error) {
	var evt storedDomainEvent
	if err := json.Unmarshal([]byte(payload), &evt); err != nil {
		return nil, err
	}
	return evt, nil
}
